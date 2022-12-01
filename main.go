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
	sources := make([]string, 0)
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
	if strings.Contains(messageStr, "Invalid HTTP request received.") || strings.Contains(messageStr, "GET /metrics HTTP") || strings.Contains(messageStr, "GET /health_check") {
		return true
	}
	log.Print(messageStr)
	return true
}

func main() {
	log.SetFlags(0)
	conf := initConf()
	sources := getAllSources(conf)
	defaultSource := ""

	if len(sources) > 0 {
		defaultSource = sources[0]
	}

	addEnvFlag := flag.Bool("a", false, "新增项目")
	deployment := flag.String("d", "", "项目名")
	env := flag.String("e", "dev", "环境选择：dev | prod")
	numberOfLines := flag.Int("l", -1, "显示多少行")
	name := flag.String("n", "", "服务名")
	namespace := flag.String("ns", "", "命名空间：如 dev1")
	refreshTokenFlag := flag.Bool("r", false, "刷新 token")
	source := flag.String("s", defaultSource, fmt.Sprintf(`日志来源，即配置文件中的别名/Source of env in $HOME/.kkconfig.yaml %v`, sources))
	_type := flag.String("t", "api", "服务类型: api | script")

	flag.Parse()
	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	if *addEnvFlag {
		addEnv()
	}

	if *refreshTokenFlag {
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

	curConf, ok := conf.EnvMap[*source]
	if !ok {
		log.Printf(`日志来源[ %v ]未定义，请检查`, *source)
		curConf = &Env{}
	}

	if *namespace != "" {
		curConf.Namespace = *namespace
	}
	if *deployment != "" {
		curConf.Deployment = *deployment
	}
	if *name != "" {
		curConf.Name = *name
	}
	if *_type != "" {
		curConf.Type = *_type
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
		"namespace=" + curConf.Namespace,
		"label=app=" + curConf.Deployment + ",cicd_env=stable,name=" + curConf.Name + ",type=" + curConf.Type + ",version=stable",
	}

	var link = `wss://value.weike.fm/ws/api/k8s/` + *env + `/pods/log`
	link += "?" + strings.Join(args, "&")
	log.Printf("Connecting:[%s]\nNamespace:[%s]\nLink:[%s]", *source, *namespace, link)
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
