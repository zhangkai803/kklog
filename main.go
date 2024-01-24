package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/websocket"
)

func getAllSources(c *Conf) []string {
    sources := make([]string, 0)
    for k := range c.Envs {
        sources = append(sources, c.Envs[k].Source)
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
    // 过滤一些无意义日志
    if (strings.Contains(messageStr, "Invalid HTTP request received.") ||
        strings.Contains(messageStr, "GET /metrics HTTP") ||
        strings.Contains(messageStr, "/health_check")) {
        return true
    }
    log.Print(messageStr)
    return true
}

var PROJECT_ID_MAP = map[string]string{
    "weike": "1",
    "dayou": "40",
}

func main() {
    log.SetFlags(0)
    conf := initConf()
    sources := getAllSources(conf)

    addEnvFlag := flag.Bool("a", false, "新增项目")
    deployment := flag.String("d", "", "项目名")
    debug := flag.Bool("debug", false, "DEBUG: 输出 WSS 路径但不进行连接")
    env := flag.String("e", "dev", "环境选择: dev | prod")
    tailLines := flag.String("l", "500", "tail行数: 500 | 1000 | 2000]")
    name := flag.String("n", "", "服务名")
    namespace := flag.String("ns", "", "命名空间：如 dev1")
    refreshTokenFlag := flag.Bool("r", false, "刷新 token")
    source := flag.String("s", "", fmt.Sprintf(`日志来源，即配置文件中的别名/Source of env in $HOME/.kkconfig.yaml %v`, sources))
    _type := flag.String("t", "api", "服务类型: api | script")
    project := flag.String("p", "weike", "项目区分: weike | dayou")

    projectId, getPidOK := PROJECT_ID_MAP[*project]
    if (!getPidOK) {
        log.Printf(`项目[ %v ]不存在，请检查\n`, *project)
        os.Exit(1)
    }

    flag.Parse()
    if len(os.Args) < 2 {
        flag.Usage()
        os.Exit(0)
    }

    if *addEnvFlag {
        addEnv()
        os.Exit(0)
    }

    if *refreshTokenFlag {
        refreshToken()
        os.Exit(0)
    }

    // 监听主动退出信号
    interrupt := make(chan os.Signal, 1)
    signal.Notify(interrupt, os.Interrupt)

    // 用于标记某事做完的常用做法 空结构体管道
    done := make(chan struct{})

    // 计时器 定时回复消息 ping wss server
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    var curConf *Env
    var ok bool
    if len(*source) > 0 {
        // 传入日志源
        curConf, ok = conf.EnvMap[*source]
        if !ok {
            log.Printf(`日志来源[ %v ]未定义，请检查\n`, *source)
            os.Exit(0)
        }
    } else {
        // 未传日志源
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
    if curConf.Type == "" {
        curConf.Type = *_type
    }

    // 组装地址
    args := []string{
        "container=app",
        "follow=true",
        "previous=false",
        "timestamps=true",
        "prefix=false",
        "tailLines=" + *tailLines,
        "proj_id=" + projectId,
        "token=" + conf.User.Token,
        "namespace=" + curConf.Namespace,
        "label=app=" + curConf.Deployment + ",cicd_env=stable,name=" + curConf.Name + ",type=" + curConf.Type + ",version=stable",
    }

    var link = `wss://value.weike.fm/ws/api/k8s/` + *env + `/pods/log`
    link += "?" + strings.Join(args, "&")
    log.Printf("Connecting:[%s]\nNamespace:[%s]\nLink:[%s]", curConf.Name, curConf.Namespace, link)
    // 建立 ws 连接
    c, resp, err := websocket.DefaultDialer.Dial(link, nil)
    if err != nil {
        token, err := jwt.Parse(conf.User.Token, func(token *jwt.Token)(any, error){
            fmt.Printf("token: %v\n", token)
            return []byte("somekey"), nil
        })
        if err := token.Claims.Valid(); err != nil {
            log.Printf("效能平台 token 失效\n")
            os.Exit(1)
        }
        log.Printf("Websocket 连接失败，请检查参数 err: %s\n", err.Error())
        os.Exit(1)
    }

    defer func(c *websocket.Conn) {
        err := c.Close()
        if err != nil {
            fmt.Println("Close websocket error", err)
        }
    }(c)

    if *debug {
        log.Printf("Connect resp: %v\n", resp.Status)
        os.Exit(0)
    }

    // goroutine 读取消息
    go func() {
        defer close(done)
        for {
            r := handleMessage(c)
            if !r {
                break
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
