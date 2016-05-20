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


	if(len(c.SrcEs)==0&&len(c.DumpInputFile)==0){
		log.Error("no input, type --help for more details")
		return
	}
	if(len(c.DestEs)==0&&len(c.DumpOutFile)==0){
		log.Error("no output, type --help for more details")
		return
	}


	// enough of a buffer to hold all the search results across all workers
	c.DocChan = make(chan map[string]interface{}, c.DocBufferCount*c.Workers*10)


	//dealing with basic auth
	if(len(c.SrcEsAuthStr)>0&&strings.Contains(c.SrcEsAuthStr,":")){
		authArray:=strings.Split(c.SrcEsAuthStr,":")
		auth:=Auth{User:authArray[0],Pass:authArray[1]}
		c.SrcAuth=&auth
	}
	if(len(c.DestEsAuthStr)>0&&strings.Contains(c.DestEsAuthStr,":")){
		authArray:=strings.Split(c.DestEsAuthStr,":")
		auth:=Auth{User:authArray[0],Pass:authArray[1]}
		c.DescAuth=&auth
	}

	//get source es version
	srcESVersion, errs := c.ClusterVersion(c.SrcEs,c.SrcAuth)
	if errs != nil {
		return
	}

	if strings.HasPrefix(srcESVersion.Version.Number,"5.") {
		log.Debug("src es is V5,",srcESVersion.Version.Number)
		api:=new(ESAPIV5)
		api.Host=c.SrcEs
		api.Auth=c.SrcAuth
		c.SrcESAPI = api
	} else {
		log.Debug("src es is not V5,",srcESVersion.Version.Number)
		api:=new(ESAPIV0)
		api.Host=c.SrcEs
		api.Auth=c.SrcAuth
		c.SrcESAPI = api
	}

	//get target es version
	descESVersion, errs := c.ClusterVersion(c.DestEs,c.DescAuth)
	if errs != nil {
		return
	}

	if strings.HasPrefix(descESVersion.Version.Number,"5.") {
		log.Debug("dest es is V5,",descESVersion.Version.Number)
		api:=new(ESAPIV5)
		api.Host=c.DestEs
		api.Auth=c.DescAuth
		c.DescESAPI = api
	} else {
		log.Debug("dest es is not V5,",descESVersion.Version.Number)
		api:=new(ESAPIV0)
		api.Host=c.DestEs
		api.Auth=c.DescAuth
		c.DescESAPI = api

	}

	// get all indexes from source
	indexNames,indexCount, srcIndexMappings,err := c.SrcESAPI.GetIndexMappings(c.CopyAllIndexes,c.SrcIndexNames);
	if(err!=nil){
		log.Error(err)
		return
	}

	//override indexnames to be copy
	c.SrcIndexNames=indexNames

	// wait for cluster state to be okay before moving
	timer := time.NewTimer(time.Second * 3)

	for {
		if status, ready := c.ClusterReady(c.SrcESAPI); !ready {
			log.Infof("%s at %s is %s, delaying move ", status.Name, c.SrcEs, status.Status)
			<-timer.C
			continue
		}
		if status, ready := c.ClusterReady(c.DescESAPI); !ready {
			log.Infof("%s at %s is %s, delaying move ", status.Name, c.DestEs, status.Status)
			<-timer.C
			continue
		}

		timer.Stop()
		break
	}

	// copy index settings if user asked
	if(c.CopyIndexSettings||c.ShardsCount>0){
		log.Info("start moving settings/mappings..")

		var srcIndexSettings *Indexes
		if(c.CopyIndexSettings){
			srcIndexSettings,err = c.SrcESAPI.GetIndexSettings(indexNames)
			log.Debug("src index settings:",srcIndexSettings)

			if err != nil {
				log.Error(err)
				return
			}
		}else{
			srcIndexSettings=&Indexes{}
			//TODO
			//it seems we need to reshard
			for name := range *srcIndexMappings {
				//srcIndexSettings.SetShardCount(name, fmt.Sprint(c.ShardsCount))
				log.Debug(name)
			}
		}


		// overwrite index shard settings
		if c.ShardsCount > 0 {
			for name := range *srcIndexSettings {
				srcIndexSettings.SetShardCount(name, fmt.Sprint(c.ShardsCount))
			}
		}

		//if there is only one index and we specify the dest indexname
		if((c.SrcIndexNames!=c.DestIndexName)&&(indexCount==1||(len(c.DestIndexName)>0))){
			log.Debug("only one index,so we can rewrite indexname")
			(*srcIndexSettings)[c.DestIndexName]=(*srcIndexSettings)[c.SrcIndexNames]
			delete(*srcIndexSettings,c.SrcIndexNames)
			log.Debug(srcIndexSettings)
		}

		//copy indexsettings and mappings
		err=c.DescESAPI.CreateIndexes(srcIndexSettings)
		if err != nil {
			log.Error(err)
		}

		//c.CreateMappings(srcIndexMappings)
		log.Info("settings/mappings move finished.")
	}

	log.Info("start moving data..")

	// start scroll
	scroll, err := c.SrcESAPI.NewScroll(c.SrcIndexNames,c.ScrollTime,c.DocBufferCount)
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

	log.Info("data move finished.")

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

func (idxs *Indexes) SetShardCount(indexName, shards string) {

	index := (*idxs)[indexName]
	if _, ok := (*idxs)[indexName].(map[string]interface{})["settings"]; !ok {
		index.(map[string]interface{})["settings"] = map[string]interface{}{}
	}

	if _, ok := (*idxs)[indexName].(map[string]interface{})["settings"].(map[string]interface{})["index"]; !ok {
		index.(map[string]interface{})["settings"].(map[string]interface{})["index"] = map[string]interface{}{}
	}

	index.(map[string]interface{})["settings"].(map[string]interface{})["index"].(map[string]interface{})["number_of_shards"] = shards

	//clean up settings
	delete(index.(map[string]interface{})["settings"].(map[string]interface{})["index"].(map[string]interface{}),"creation_date")
	delete(index.(map[string]interface{})["settings"].(map[string]interface{})["index"].(map[string]interface{}),"uuid")
	delete(index.(map[string]interface{})["settings"].(map[string]interface{})["index"].(map[string]interface{}),"version")
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
