package main

import (
	"bytes"
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
	gorequest "github.com/parnurzeal/gorequest"
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

	//get source es version
	srcESVersion, errs := c.ClusterVersion(c.SrcEs)
	if errs != nil {
		return
	}

	if strings.HasPrefix(srcESVersion.Version.Number,"5.") {
		log.Debug("src es is V5,",srcESVersion.Version.Number)
		api:=new(ESAPIV5)
		api.Host=c.SrcEs
		c.SrcESAPI = api
	} else {
		log.Debug("src es is not V5,",srcESVersion.Version.Number)
		api:=new(ESAPIV0)
		api.Host=c.SrcEs
		c.SrcESAPI = api
	}

	//get target es version
	descESVersion, errs := c.ClusterVersion(c.DstEs)
	if errs != nil {
		return
	}

	if strings.HasPrefix(descESVersion.Version.Number,"5.") {
		log.Debug("dest es is V5,",descESVersion.Version.Number)
		api:=new(ESAPIV5)
		api.Host=c.DstEs
		c.DescESAPI = api
	} else {
		log.Debug("dest es is not V5,",descESVersion.Version.Number)
		api:=new(ESAPIV0)
		api.Host=c.DstEs
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

	// disable replication in settings
	if c.EnableReplication == false {
		idxs.DisableReplication()
	}

	if c.IndexDocsOnly == false {
		// delete remote indexes if user asked
		if c.Destructive == true {
			if err := c.DeleteIndexes(idxs); err != nil {
				log.Error(err)
				return
			}
		}

		// create indexes on DstEs
		if err := c.CreateIndexes(idxs); err != nil {
			log.Error(err)
			return
		}
	}

	// if we only want to create indexes, we are done here, return
	if c.CreateIndexesOnly {
		log.Info("Indexes created, done")
		return
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
		fetchBar := pb.New(scroll.Hits.Total).Prefix("Fetch ")
		bulkBar := pb.New(scroll.Hits.Total).Prefix("Bulk ")

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
			go c.NewWorker(&docCount, bulkBar, &wg)
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

}

func setInitLogging(logLevel string) {

	logLevel = strings.ToLower(logLevel)

	testConfig := `
	<seelog  type="sync" minlevel="`
	testConfig = testConfig + logLevel
	testConfig = testConfig + `">
		<outputs formatid="main">
			<filter levels="error">
				<file path="./log/gopa.log"/>
			</filter>
			<console formatid="main" />
		</outputs>
		<formats>
			<format id="main" format="[%Date(01-02) %Time] [%LEV] [%File:%Line,%FuncShort] %Msg%n"/>
		</formats>
	</seelog>`

	logger, err := log.LoggerFromConfigAsString(testConfig)
	if err != nil {
		log.Error("init config error,", err)
	}
	err = log.ReplaceLogger(logger)
	if err != nil {
		log.Error("init config error,", err)
	}
}

// Stream from source es instance. "done" is an indicator that the stream is
// over
func (s *Scroll) ProcessScrollResult(c *Config, bar *pb.ProgressBar){

	//update progress bar
	bar.Add(len(s.Hits.Docs))

	// show any failures
	for _, failure := range s.Shards.Failures {
		reason, _ := json.Marshal(failure.Reason)
		log.Errorf(string(reason))
	}

	// write all the docs into a channel
	for _, docI := range s.Hits.Docs {
		c.DocChan <- docI.(map[string]interface{})
	}
}

func (s *Scroll) Next(c *Config, bar *pb.ProgressBar) (done bool) {

	scroll,err:=c.SrcESAPI.NextScroll(c.ScrollTime,s.ScrollId)
	if err != nil {
		log.Error(err)
		return false
	}

	if scroll.Hits.Docs == nil || len(scroll.Hits.Docs) <= 0 {
		log.Debug("scroll result is empty")
		return true
	}

	scroll.ProcessScrollResult(c,bar)

	//update scrollId
	s.ScrollId=scroll.ScrollId

	return
}

func (c *Config) NewWorker(docCount *int, pb *pb.ProgressBar, wg *sync.WaitGroup) {

	bulkItemSize := 0
	mainBuf := bytes.Buffer{}
	docBuf := bytes.Buffer{}
	docEnc := json.NewEncoder(&docBuf)

READ_DOCS:
	for {
		var err error
		docI, open := <-c.DocChan

		// this check is in case the document is an error with scroll stuff
		if status, ok := docI["status"]; ok {
			if status.(int) == 404 {
				log.Error("error: ", docI["response"])
				continue
			}
		}

		// sanity check
		for _, key := range []string{"_index", "_type", "_source", "_id"} {
			if _, ok := docI[key]; !ok {
				//json,_:=json.Marshal(docI)
				//log.Errorf("failed parsing document: %v", string(json))
				break READ_DOCS
			}
		}

		var tempDestIndexName string
		tempDestIndexName = docI["_index"].(string)

		if c.DestIndexName != "" {
			tempDestIndexName = c.DestIndexName
		}

		doc := Document{
			Index:  tempDestIndexName,
			Type:   docI["_type"].(string),
			source: docI["_source"].(map[string]interface{}),
			Id:     docI["_id"].(string),
		}

		// if channel is closed flush and gtfo
		if !open {
			goto WORKER_DONE
		}

		// sanity check
		if len(doc.Index) == 0 || len(doc.Id) == 0 || len(doc.Type) == 0 {
			log.Errorf("failed decoding document: %+v", doc)
			continue
		}

		// encode the doc and and the _source field for a bulk request
		post := map[string]Document{
			"create": doc,
		}
		if err = docEnc.Encode(post); err != nil {
			log.Error(err)
		}
		if err = docEnc.Encode(doc.source); err != nil {
			log.Error(err)
		}

		// if we approach the 100mb es limit, flush to es and reset mainBuf
		if mainBuf.Len()+docBuf.Len() > (c.BulkSizeInMB * 1000000) {
			c.DescESAPI.Bulk(&mainBuf)
			pb.Add(bulkItemSize)
			bulkItemSize = 0
		}

		// append the doc to the main buffer
		mainBuf.Write(docBuf.Bytes())
		// reset for next document
		bulkItemSize++
		docBuf.Reset()
		(*docCount)++
	}

WORKER_DONE:
	if docBuf.Len() > 0 {
		mainBuf.Write(docBuf.Bytes())
		bulkItemSize++
	}
	c.DescESAPI.Bulk(&mainBuf)
	pb.Add(bulkItemSize)
	bulkItemSize = 0
	wg.Done()
}

func (c *Config)ClusterVersion(host string) (*ClusterVersion, []error) {

	url := fmt.Sprintf("%s", host)
	_, body, errs := Get(url)
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


// CreateIndexes on remodeleted ES instance
func (c *Config) CreateIndexes(idxs *Indexes) (err error) {

	for name, idx := range *idxs {
		body := bytes.Buffer{}
		enc := json.NewEncoder(&body)
		enc.Encode(idx)

		resp, err := http.Post(fmt.Sprintf("%s/%s", c.DstEs, name), "", &body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			b, _ := ioutil.ReadAll(resp.Body)
			return fmt.Errorf("failed creating index: %s", string(b))
		}

		log.Info("created index: ", name)
	}

	return
}

func (c *Config) DeleteIndexes(idxs *Indexes) (err error) {

	for name, idx := range *idxs {
		body := bytes.Buffer{}
		enc := json.NewEncoder(&body)
		enc.Encode(idx)

		req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", c.DstEs, name), nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			// thats okay, index doesnt exist
			continue
		}

		if resp.StatusCode != 200 {
			b, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("failed deleting index: %s", string(b))
		}

		log.Error("deleted index: ", name)
	}

	return
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

func (idxs *Indexes) DisableReplication() {

	for name, index := range *idxs {
		if _, ok := (*idxs)[name].(map[string]interface{})["settings"]; !ok {
			index.(map[string]interface{})["settings"] = map[string]interface{}{}
		}

		if _, ok := (*idxs)[name].(map[string]interface{})["settings"].(map[string]interface{})["index"]; !ok {
			index.(map[string]interface{})["settings"].(map[string]interface{})["index"] = map[string]interface{}{}
		}

		index.(map[string]interface{})["settings"].(map[string]interface{})["index"].(map[string]interface{})["number_of_replicas"] = "0"
	}
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


func Get(url string) (*http.Response, string, []error) {
	request := gorequest.New() //.SetBasicAuth("username", "password")

	resp, body, errs := request.Get(url).End()

	return resp, body, errs

	//reuse
	//resp, body, errs := gorequest.New().Get("http://example.com/").End()

}

func Post(url string, body []byte) {
	request := gorequest.New() //.SetBasicAuth("username", "password")

	//resp, body, errs :=
	request.Post(url).Send(body).End()
}

