package main

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v8"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//一个git事件对象
type ProjectMes struct {
	ProjectName string `json:"project_name"`
	UserName    string `json:"user_name"`
	Description string `json:"description"`
	Branch      string `json:"branch"`
	Version     int64  `json:"version"`
	Commit      string `json:"commit"`
}

//jenkins查询构建结果返回结构体
type BuildResult struct {
	Class  string `json:"_class"`
	Result string `json:"result"`
	Id     string `json:"id"`
}

//一个构建结果
type BuildRecord struct {
	ProjectName string `json:"project_name"`
	UserName    string `json:"user_name"`
	Result      string `json:"result"`
	NowVersion  int64  `json:"now_version"`
}

//var redispw = "CityDo@123"
//保存本地版本号用来校验---后续眼保存到redis中
var Version int64 = 0
var redispw = "123456"

//http客户端
var Client = http.Client{}
var LastBuild = make([]map[string]int64, 0)

//从redis取出构建需要的信息
func GetData() {
	spec := make([]map[string]ProjectMes, 0)
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "123456", // no password set
		DB:       0,        // use default DB
	})
	var ctx = context.Background()
	data1, err := rdb.Get(ctx, "GitMes").Result()
	if err != nil {
		log.Printf("查询redis错误err:%v", err)
	}
	err = json.Unmarshal([]byte(data1), &spec)
	if err != nil {
		log.Printf("json反序列化错误err: %v", err)
	}
	for _, v := range spec {
		for _, p := range v {
			log.Println("开始对比版本")
			//从redis中刚查询客户端执行版本
			rversion, err := rdb.Get(ctx, "ClientVersion").Result()
			if err != nil {
				log.Printf("查询redis错误err:%v", err)
			}
			clientversion, err := strconv.ParseInt(rversion, 10, 64)
			if err != nil {
				log.Printf("转换失败")
			}
			log.Printf("从redis取出版本为%v", clientversion)
			//如何redis中客户端执行版本高于当前记录版本，则将当前记录版本升级为redis中客户端执行版本
			if clientversion > Version {
				Version = clientversion
			}
			//对比客户端执行版本和服务端执行版本，如果客户端高则无需执行动作
			if Version >= p.Version {
				log.Printf("当期项目:%v,版本为%v,历史版本为%v,无需更新", p.ProjectName, p.Version, Version)
				continue
			}
			log.Printf("%v在%v项目(%v)的%v分支进行了push", p.UserName, p.ProjectName, p.Description, p.Branch)
			go Build(p.Branch, p.ProjectName, p.Version, p.UserName)
		}
	}
}

//调用jenkins构建

func Build(brach string, projectName string, version int64, user string) {
	log.Println("开始构建")
	//拼凑构建请求
	req, err := http.NewRequest("POST", "http://127.0.0.1:8080/job/"+projectName+"/buildWithParameters", strings.NewReader("name"+"="+brach))
	defer req.Body.Close()
	if err != nil {
		log.Printf("http请求错误err:%v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("admin", "admin")
	Client.Do(req)
	//执行完毕后更新当前执行version
	if version > Version {
		Version = version
	}
	//保存当前执行版本到redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "123456", // no password set
		DB:       0,        // use default DB
	})
	var ctx = context.Background()
	err = rdb.Set(ctx, "ClientVersion", Version, 0).Err()
	//_, err = redis.Do("set", "GitMes", s)
	if err != nil {
		log.Println("插入ServerVersion错误err:", err)
	}
	//等待构建完成，这里是不是可以通过查询当前构建队列获知进度呢，定时不太好
	time.Sleep(time.Second * 5)
	//查询构建结果
	req, err = http.NewRequest("GET", "http://127.0.0.1:8080/job/"+projectName+"/lastBuild/api/json?pretty=true&tree=result,id", nil)
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
	buildResult := &BuildResult{}
	err = json.Unmarshal(body, buildResult)
	if err != nil {
		log.Printf("反序列化构建结果消息失败err:%v", err)
	}
	//req, err = http.NewRequest("GET", "http://127.0.0.1:8080/job/"+projectName+"/lastBuild/logText/progressiveText", nil)
	req, err = http.NewRequest("GET", "http://127.0.0.1:8080/job/222222/lastBuild/buildNumber", nil)
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
	//对比最后一次构建的id和最后一次构建成功的id是否一致
	if string(body) == buildResult.Id {
		log.Println(string(body), buildResult.Id)
		//构建build结果消息
		buildRecord := BuildRecord{
			ProjectName: projectName,
			UserName:    user,
			Result:      buildResult.Result,
			NowVersion:  version,
		}
		//序列化
		data, err := json.Marshal(buildRecord)
		//调用server mes接口发送钉钉消息
		req, err = http.NewRequest("POST", "http://127.0.0.1:8182/mes", strings.NewReader(string(data)))
		if err != nil {
			log.Printf("构建请求体失败err:%v", err)
		}
		Client.Do(req)
	} else {
		log.Println("等待build结束")
	}
}
func main() {
	//定时轮询redis
	ticker := time.NewTicker(time.Second * 10)
	log.Println("开始定时任务60s")
	for _ = range ticker.C {
		log.Println("检查是否有pushevent")
		GetData()
	}
}
