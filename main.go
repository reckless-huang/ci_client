package main

import (
	"encoding/json"
	"github.com/garyburd/redigo/redis"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

type ProjectMes struct {
	ProjectName string `json:"project_name"`
	UserName    string `json:"user_name"`
	Description string `json:"description"`
	Branch      string `json:"branch"`
	Version     int64  `json:"version"`
	Commit      string `json:"commit"`
}

type BuildResult struct {
	Class  string `json:"_class"`
	Result string `json:"result"`
}

type BuildRecord struct {
	ProjectName string `json:"project_name"`
	UserName    string `json:"user_name"`
	Result      string `json:"result"`
}

//var redispw = "CityDo@123"
var Version int64 = 0
var redispw = "123456"
var Client = http.Client{}
var LastBuild = make([]map[string]int64, 0)
var RedisPool = &redis.Pool{
	Dial: func() (redis.Conn, error) {
		//c, err := redis.Dial("tcp", "47.111.20.132:6379")
		c, err := redis.Dial("tcp", "127.0.0.1:6379")
		if err != nil {
			log.Fatalf("初始化redis连接池异常err:", err)
		}
		_, err = c.Do("AUTH", redispw)
		if err != nil {
			log.Fatalf("redis连接池验证异常err:", err)
		}
		return c, err
	},
	MaxActive:   100,
	MaxIdle:     10,
	IdleTimeout: 240 * time.Second,
	Wait:        true,
}

func GetData() {
	redis := RedisPool.Get()
	defer redis.Close()
	spec := make([]map[string]ProjectMes, 0)
	data, err := redis.Do("get", "GitMes")
	//log.Println(data)
	//data1 := data.([]uint8)
	switch data.(type) {
	case []uint8:
		data1 := data.([]uint8)
		err := json.Unmarshal(data1, &spec)
		if err != nil {
			log.Printf("json反序列化错误err: %v", err)
		}
		for _, v := range spec {
			for _, p := range v {
				log.Println("开始对比版本")
				if Version >= p.Version {
					log.Printf("当期项目:%v,版本为%v,历史版本为%v,无需更新", p.ProjectName, p.Version, Version)
					continue
				}
				log.Printf("%v在%v项目(%v)的%v分支进行了push", p.UserName, p.ProjectName, p.Description, p.Branch)
				go Build(p.Branch, p.ProjectName, p.Version)
			}
		}
	default:
		log.Println("没有pushevent")
	}
	if err != nil {
		log.Println("从redis中获取数据错误err:", err)
	}
}
func Build(brach string, projectName string, version int64) {
	buildResult := &BuildResult{}
	log.Println("开始构建")
	req, err := http.NewRequest("POST", "http://127.0.0.1:8080/job/"+projectName+"/buildWithParameters", strings.NewReader("name"+"="+brach))
	defer req.Body.Close()
	if err != nil {
		log.Printf("http请求错误err:%v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("admin", "admin")
	Client.Do(req)
	time.Sleep(time.Second * 5)
	req, err = http.NewRequest("GET", "http://127.0.0.1:8080/job/"+projectName+"/lastBuild/api/json?pretty=true&tree=result", nil)
	if err != nil {
		log.Printf("构建请求体失败err:%v", err)
	}
	req.SetBasicAuth("admin", "admin")
	res, err := Client.Do(req)
	if err != nil {
		log.Printf("查询该项目最后一次构建失败err:%v", err)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("从body中读取内容错误err:%v", err)
	}
	err = json.Unmarshal(body, buildResult)
	if err != nil {
		log.Printf("反序列化构建结果消息失败err:%v", err)
	}
	if version > Version {
		Version = version
	}
	req, err = http.NewRequest("GET", "http://127.0.0.1:8080/job/"+projectName+"/lastBuild/logText/progressiveText", nil)
	if err != nil {
		log.Printf("构建请求体失败err:%v", err)
	}
	req.SetBasicAuth("admin", "admin")
	res, err = Client.Do(req)
	if err != nil {
		log.Printf("查询该项目最后一次构建失败err:%v", err)
	}
	body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("从body中读取内容错误err:%v", err)
	}
	log.Println(string(body))
}
func main() {
	ticker := time.NewTicker(time.Second * 10)
	log.Println("开始定时任务60s")
	for _ = range ticker.C {
		log.Println("检查是否有pushevent")
		GetData()
	}
}
