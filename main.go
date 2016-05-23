package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	goflags "github.com/jessevdk/go-flags"
	pb "gopkg.in/cheggaaa/pb.v1"
)

func main() {

	runtime.GOMAXPROCS(runtime.NumCPU())

	c := Config{
		FlushLock: sync.Mutex{},
	}

	// parse args
	_, err := goflags.Parse(&c)
	if err != nil {
		log.Error(err)
		return
	}

	setInitLogging(c.LogLevel)


	if(len(c.SourceEs)==0&&len(c.DumpInputFile)==0){
		log.Error("no input, type --help for more details")
		return
	}
	if(len(c.TargetEs)==0&&len(c.DumpOutFile)==0){
		log.Error("no output, type --help for more details")
		return
	}


	// enough of a buffer to hold all the search results across all workers
	c.DocChan = make(chan map[string]interface{}, c.DocBufferCount*c.Workers*10)


	//dealing with basic auth
	if(len(c.SourceEsAuthStr)>0&&strings.Contains(c.SourceEsAuthStr,":")){
		authArray:=strings.Split(c.SourceEsAuthStr,":")
		auth:=Auth{User:authArray[0],Pass:authArray[1]}
		c.SourceAuth =&auth
	}
	if(len(c.TargetEsAuthStr)>0&&strings.Contains(c.TargetEsAuthStr,":")){
		authArray:=strings.Split(c.TargetEsAuthStr,":")
		auth:=Auth{User:authArray[0],Pass:authArray[1]}
		c.TargetAuth =&auth
	}

	//get source es version
	srcESVersion, errs := c.ClusterVersion(c.SourceEs,c.SourceAuth)
	if errs != nil {
		return
	}

	if strings.HasPrefix(srcESVersion.Version.Number,"5.") {
		log.Debug("source es is V5,",srcESVersion.Version.Number)
		api:=new(ESAPIV5)
		api.Host=c.SourceEs
		api.Auth=c.SourceAuth
		c.SourceESAPI = api
	} else {
		log.Debug("source es is not V5,",srcESVersion.Version.Number)
		api:=new(ESAPIV0)
		api.Host=c.SourceEs
		api.Auth=c.SourceAuth
		c.SourceESAPI = api
	}

	//get target es version
	descESVersion, errs := c.ClusterVersion(c.TargetEs,c.TargetAuth)
	if errs != nil {
		return
	}

	if strings.HasPrefix(descESVersion.Version.Number,"5.") {
		log.Debug("target es is V5,",descESVersion.Version.Number)
		api:=new(ESAPIV5)
		api.Host=c.TargetEs
		api.Auth=c.TargetAuth
		c.TargetESAPI = api
	} else {
		log.Debug("target es is not V5,",descESVersion.Version.Number)
		api:=new(ESAPIV0)
		api.Host=c.TargetEs
		api.Auth=c.TargetAuth
		c.TargetESAPI = api

	}

	// wait for cluster state to be okay before moving
	timer := time.NewTimer(time.Second * 3)

	for {
		if status, ready := c.ClusterReady(c.SourceESAPI); !ready {
			log.Infof("%s at %s is %s, delaying migration ", status.Name, c.SourceEs, status.Status)
			<-timer.C
			continue
		}
		if status, ready := c.ClusterReady(c.TargetESAPI); !ready {
			log.Infof("%s at %s is %s, delaying migration ", status.Name, c.TargetEs, status.Status)
			<-timer.C
			continue
		}

		timer.Stop()
		break
	}


	// get all indexes from source
	indexNames,indexCount, srcIndexMappings,err := c.SourceESAPI.GetIndexMappings(c.CopyAllIndexes,c.SourceIndexNames);
	if(err!=nil){
		log.Error(err)
		return
	}

	if(indexCount>0){
		//override indexnames to be copy
		c.SourceIndexNames =indexNames

		// copy index settings if user asked
		if(c.CopyIndexSettings||c.ShardsCount>0){
			log.Info("start settings/mappings migration..")

			//get source index settings
			var sourceIndexSettings *Indexes
			sourceIndexSettings,err := c.SourceESAPI.GetIndexSettings(c.SourceIndexNames)
			log.Debug("source index settings:", sourceIndexSettings)
			if err != nil {
				log.Error(err)
				return
			}

			//get target index settings
			targetIndexSettings,err := c.TargetESAPI.GetIndexSettings(c.TargetIndexName);
			if(err!=nil){
				//ignore target es settings error
				log.Debug(err)
			}
			log.Debug("target IndexSettings", targetIndexSettings)


			//if there is only one index and we specify the dest indexname
			if((c.SourceIndexNames !=c.TargetIndexName)&&(indexCount==1||(len(c.TargetIndexName)>0))){
				log.Debug("only one index,so we can rewrite indexname")
				(*sourceIndexSettings)[c.TargetIndexName]=(*sourceIndexSettings)[c.SourceIndexNames]
				delete(*sourceIndexSettings,c.SourceIndexNames)
				log.Debug(sourceIndexSettings)
			}

			// dealing with indices settings
			for name, idx := range *sourceIndexSettings {
				log.Debug("dealing with index,name:",name,",settings:",idx)
				tempIndexSettings:=map[string]interface{}{}
				tempIndexSettings["settings"] = map[string]interface{}{}

				targetIndexExist:=false
				//if target index settings is exist and we don't copy settings, we use target settings
				if(targetIndexSettings!=nil){
					//if target es have this index and we dont copy index settings
					if val, ok := (*targetIndexSettings)[name]; ok {
						targetIndexExist=true
						tempIndexSettings=val.(map[string]interface{})
					}

					if(c.RecreateIndex){
						c.TargetESAPI.DeleteIndex(name)
						targetIndexExist=false
					}
				}

				//copy index settings
				if(c.CopyIndexSettings){
					tempIndexSettings=((*sourceIndexSettings)[name]).(map[string]interface{})
				}

				//check map elements
				if _, ok := tempIndexSettings["settings"]; !ok {
					tempIndexSettings["settings"] = map[string]interface{}{}
				}

				if _, ok := tempIndexSettings["settings"].(map[string]interface{})["index"]; !ok {
					tempIndexSettings["settings"].(map[string]interface{})["index"] = map[string]interface{}{}
				}


				//set refresh_interval
				tempIndexSettings["settings"].(map[string]interface{})["index"].(map[string]interface{})["refresh_interval"] = -1
				tempIndexSettings["settings"].(map[string]interface{})["index"].(map[string]interface{})["number_of_replicas"] = 0

				//clean up settings
				delete(tempIndexSettings["settings"].(map[string]interface{})["index"].(map[string]interface{}),"number_of_shards")


				//copy indexsettings and mappings
				if(targetIndexExist){
					log.Debug("update index with settings,",name,tempIndexSettings)
					err:=c.TargetESAPI.UpdateIndexSettings(name,tempIndexSettings)
					if err != nil {
						log.Error(err)
					}
				}else{

					//override shard settings
					if(c.ShardsCount>0){
						tempIndexSettings["settings"].(map[string]interface{})["index"].(map[string]interface{})["number_of_shards"] = c.ShardsCount
					}

					log.Debug("create index with settings,",name,tempIndexSettings)
					err:=c.TargetESAPI.CreateIndex(name,tempIndexSettings)
					if err != nil {
						log.Error(err)
					}
				}

				//TODO c.CreateMappings(srcIndexMappings)
				log.Debug(srcIndexMappings)
			}

			log.Info("settings/mappings migration finished.")
		}


	}else{
		log.Error("Index not exists,",c.SourceIndexNames)
		return
	}


	log.Info("start data migration..")

	// start scroll
	scroll, err := c.SourceESAPI.NewScroll(c.SourceIndexNames,c.ScrollTime,c.DocBufferCount)
	if err != nil {
		log.Error(err)
		return
	}

	if scroll != nil && scroll.Hits.Docs != nil {
		// create a progressbar and start a docCount
		fetchBar := pb.New(scroll.Hits.Total).Prefix("Pull ")
		bulkBar := pb.New(scroll.Hits.Total).Prefix("Push ")


		// start pool
		pool, err := pb.StartPool(fetchBar, bulkBar)
		if err != nil {
			panic(err)
		}

		// update bars
		var docCount int
		wg := sync.WaitGroup{}
		wg.Add(c.Workers)
		for i := 0; i < c.Workers; i++ {
			go c.NewBulkWorker(&docCount, bulkBar, &wg)
		}

		// start file write
		if(len(c.DumpOutFile)>0){
			go func() {

			}()
		}

		scroll.ProcessScrollResult(&c,fetchBar)

		// loop scrolling until done
		for scroll.Next(&c, fetchBar) == false {
		}
		fetchBar.Finish()

		// finished, close doc chan and wait for goroutines to be done
		close(c.DocChan)
		wg.Wait()
		bulkBar.Finish()
		// close pool
		pool.Stop()
	}

	log.Info("data migration finished.")

	//TODO update replica and refresh_interval

}

func (c *Config) ClusterVersion(host string,auth *Auth) (*ClusterVersion, []error) {

	url := fmt.Sprintf("%s", host)
	_, body, errs := Get(url,auth)
	if errs != nil {
		log.Error(errs)
		return nil,errs
	}

	log.Debug(body)

	version := &ClusterVersion{}
	err := json.Unmarshal([]byte(body), version)

	if err != nil {
		log.Error(body, errs)
		return nil,errs
	}
	return version,nil
}

func (c *Config) ClusterReady(api ESAPI) (*ClusterHealth, bool) {

	health := api.ClusterHealth()
	if health.Status == "red" {
		return health, false
	}

	if c.WaitForGreen == false && health.Status == "yellow" {
		return health, true
	}

	if health.Status == "green" {
		return health, true
	}

	return health, false
}
