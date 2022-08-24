package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func getAllSources(c *Conf) []string {
	sources := make([]string, len(c.EnvMap))
	for k := range c.EnvMap {
		sources = append(sources, k)
	}
	return sources
}

func handleMessage(c *websocket.Conn) bool {
	_, message, err := c.ReadMessage()
	if err != nil {
		log.Println("read:", err)
		return false
	}
	messageStr := string(message)
	if strings.Index(messageStr, "Invalid HTTP request received.") != -1 {
		return true
	}
	log.Print(messageStr)
	return true
}

func main() {
	log.SetFlags(0)
	conf := initConf()

	source := flag.String("s", "wcm", fmt.Sprintf(`日志来源，即配置文件中的别名/Source of env in $HOME/.kkconfig.yaml %v`, getAllSources(conf)))
	namespace := flag.String("ns", "", "命名空间：如dev1,不指定默认使用配置文件里的namespace")
	numberOfLines := flag.Int("n", -1, "显示多少行")
	addEnvFlag := flag.Bool("a", false, "新增项目")
	refreshTokenFlag := flag.Bool("f", false, "刷新token")

	flag.Parse()

	if len(os.Args) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	if *addEnvFlag == true {
		addEnv()
	}

	if *refreshTokenFlag == true {
		refreshToken()
	}

	// 监听主动退出信号
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// 用于标记某事做完的常用做法 空结构体管道
	done := make(chan struct{})

	// 计时器 定时回复消息
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	_, ok := conf.EnvMap[*source]
	if !ok {
		log.Printf(`日志来源[" %v "]未定义，请检查`, *source)
		return
	}
	if *namespace == "" {
		*namespace = conf.EnvMap[*source].Namespace
	}
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
		"namespace=" + *namespace,
		"label=app=" + conf.EnvMap[*source].Deployment + ",cicd_env=stable,name=" + conf.EnvMap[*source].Name + ",type=" + conf.EnvMap[*source].Type + ",version=stable",
	}

	var link = `wss://value.weike.fm/ws/api/k8s/dev/pods/log`
	link += "?" + strings.Join(args, "&")
	log.Printf("Connecting:[%s]\nNamespace:[%s]\n", *source, *namespace)
	// 建立 ws 连接
	c, _, err := websocket.DefaultDialer.Dial(link, nil)
	if err != nil {
		log.Printf("ConnectionError!Please check your network or refresh token.\n")
		os.Exit(-1)
	}
	defer func(c *websocket.Conn) {
		err := c.Close()
		if err != nil {
			fmt.Println("Close websocket error", err)
		}
	}(c)

	// goroutine 读取消息
	go func() {
		defer close(done)
		if *numberOfLines != -1 {
			for {
				if *numberOfLines == -1 {
					os.Exit(0)
				}
				r := handleMessage(c)
				if !r {
					break
				}
				*numberOfLines = *numberOfLines - 1
			}
		} else {
			for {
				r := handleMessage(c)
				if !r {
					break
				}
			}
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
