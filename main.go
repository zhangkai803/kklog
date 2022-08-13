package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

type User struct {
	Name  string	`yaml:"name"`
	Token string	`yaml:"token"`
}

type Env struct {
	Alias 		string	`yaml:"alias"`
	Deployment 	string	`yaml:"deployment"`
	Name 		string	`yaml:"name"`
	Namespace	string	`yaml:"namespace"`
	Type 		string	`yaml:"type"`
}

type Conf struct {
        User	*User			`yaml:"user"`
        Envs	[]*Env			`yaml:"envs"`
		EnvMap  map[string]*Env
}

func getConf() *Conf {
	yamlFile, err := ioutil.ReadFile("~/.kkconf.yaml")
    if err != nil {
        log.Printf("yamlFile.Get err   #%v ", err)
    }

	c := Conf{}
    err = yaml.Unmarshal(yamlFile, &c)
    if err != nil {
        log.Fatalf("Unmarshal: %v", err)
    }

	envMap := map[string]*Env{}
	for _, e := range c.Envs {
		envMap[e.Alias] = e
	}

	c.EnvMap = envMap
	return &c
}

func main() {
	log.SetFlags(0)

	var alias string
	flag.StringVar(&alias, "alias", "wk_tag_manage", "alias of env in kkconfig")
	flag.Parse()

	// 监听主动退出信号
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// 用于标记某事做完的常用做法 空结构体管道
	done := make(chan struct{})

	// 计时器 定时回复消息
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	conf := getConf()

	// 组装地址
	args := []string{
		"container=app",
		"follow=true",
		"previous=false",
		"timestamps=true",
		"prefix=false",
		"tailLines=500",
		"proj_id=1",
		"token=" + conf.User.Token,
		"namespace=" + conf.EnvMap[alias].Namespace,
		"label=app=" + conf.EnvMap[alias].Deployment +  ",cicd_env=stable,name=" + conf.EnvMap[alias].Name + ",type=" + conf.EnvMap[alias].Type + ",version=stable",
	}

	var link string = `wss://value.weike.fm/ws/api/k8s/dev/pods/log`
	link += "?" + strings.Join(args, "&")

	log.Printf("connecting to %s", link)

	// 建立 ws 连接
	c, _, err := websocket.DefaultDialer.Dial(link, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	// gorotine 读取消息
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Print(string(message))
		}
	}()

	// 监听信号
	for {
		select {
		case <-done:
			// done and quit
			return
		case t := <-ticker.C:
			// ticker to server
			err := c.WriteMessage(websocket.TextMessage, []byte(t.String()))
			if err != nil {
				log.Println("write:", err)
				return
			}
		case <-interrupt:
			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}

			select {
			case <-done:
				// done in interrupt meaning closed and quit
			case <-time.After(time.Second):
				// after 1s force quit
			}
			return
		}
	}
}
