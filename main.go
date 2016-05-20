package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

	// enough of a buffer to hold all the search results across all workers
	c.DocChan = make(chan map[string]interface{}, c.DocBufferCount*c.Workers*10)
	c.FileChan = make(chan map[string]interface{}, c.DocBufferCount*10)


	//dealing with basic auth
	if(len(c.SrcEsAuthStr)>0&&strings.Contains(c.SrcEsAuthStr,":")){
		authArray:=strings.Split(c.SrcEsAuthStr,":")
		auth:=Auth{User:authArray[0],Pass:authArray[1]}
		c.SrcAuth=&auth
	}
	if(len(c.DescEsAuthStr)>0&&strings.Contains(c.DescEsAuthStr,":")){
		authArray:=strings.Split(c.DescEsAuthStr,":")
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
	descESVersion, errs := c.ClusterVersion(c.DstEs,c.DescAuth)
	if errs != nil {
		return
	}

	if strings.HasPrefix(descESVersion.Version.Number,"5.") {
		log.Debug("dest es is V5,",descESVersion.Version.Number)
		api:=new(ESAPIV5)
		api.Host=c.DstEs
		api.Auth=c.DescAuth
		c.DescESAPI = api
	} else {
		log.Debug("dest es is not V5,",descESVersion.Version.Number)
		api:=new(ESAPIV0)
		api.Host=c.DstEs
		api.Auth=c.DescAuth
		c.DescESAPI = api

	}

	// get all indexes from source

	indexNames,idxs,err := c.SrcESAPI.GetIndexSettings(c.CopyAllIndexes,c.SrcIndexNames);
	if(err!=nil){
		log.Error(err)
		return
	}

	c.SrcIndexNames=indexNames

	// copy index settings if user asked
	if c.CopyIndexSettings == true {
		if err := c.CopyIndexSetting(idxs); err != nil {
			log.Error(err)
			return
		}
	}

	// overwrite index shard settings
	if c.ShardsCount > 0 {
		for name := range *idxs {
			idxs.SetShardCount(name, fmt.Sprint(c.ShardsCount))
		}
	}

	if c.IndexDocsOnly == false {

		if c.DestIndexName != "" {
			//TODO
		}else{
			//// create indexes on DstEs
			//if err := c.CreateIndexes(idxs); err != nil {
			//	log.Error(err)
			//	return
			//}
		}
	}

	// wait for cluster state to be okay before moving
	timer := time.NewTimer(time.Second * 3)

	for {
		if status, ready := c.ClusterReady(c.SrcESAPI); !ready {
			log.Infof("%s at %s is %s, delaying move ", status.Name, c.SrcEs, status.Status)
			<-timer.C
			continue
		}
		if status, ready := c.ClusterReady(c.DescESAPI); !ready {
			log.Infof("%s at %s is %s, delaying move ", status.Name, c.DstEs, status.Status)
			<-timer.C
			continue
		}

		timer.Stop()
		break
	}

	log.Info("starting move..")

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

	log.Info("done move..")

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

func (c *Config) CopyIndexSetting(idxs *Indexes) (err error) {

	// get all settings
	allSettings := map[string]interface{}{}

	resp, err := http.Get(fmt.Sprintf("%s/_all/_settings", c.SrcEs))
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		return fmt.Errorf("failed getting settings for index: %s", string(b))
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&allSettings); err != nil {
		return err
	}

	for name, index := range *idxs {
		//TODO 验证 analyzer等setting是否生效
		if settings, ok := allSettings[name]; !ok {
			return fmt.Errorf("couldnt find index %s", name)
		} else {
			// omg XXX
			index.(map[string]interface{})["settings"] = map[string]interface{}{}
			var shards string
			if _, ok := settings.(map[string]interface{})["settings"].(map[string]interface{})["index"]; ok {
				// try the new style syntax first, which has an index object
				shards = settings.(map[string]interface{})["settings"].(map[string]interface{})["index"].(map[string]interface{})["number_of_shards"].(string)
			} else {
				// if not, could be running from old es, try the old style index.number_of_shards
				shards = settings.(map[string]interface{})["settings"].(map[string]interface{})["index.number_of_shards"].(string)
			}
			index.(map[string]interface{})["settings"].(map[string]interface{})["index"] = map[string]interface{}{
				"number_of_shards": shards,
			}
		}
	}

	return
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

