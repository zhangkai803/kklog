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
    for k := range c.Sources {
        sources = append(sources, c.Sources[k].Source)
    }
    return sources
}

func handleMessage(c *websocket.Conn, source *Source, grep string) bool {
    _, message, err := c.ReadMessage()
    if err != nil {
        log.Println(WrapColor(fmt.Sprintf("read error: %v", err), Red))
        return false
    }
    messageStr := string(message)
    // 过滤一些无意义日志
    if (strings.Contains(messageStr, "Invalid HTTP request received.") ||
        strings.Contains(messageStr, "GET /metrics HTTP") ||
        strings.Contains(messageStr, "/health_check")) {
        fmt.Print(".\r")
        return true
    }

    // 如果有检索内容 不匹配的内容就不会输出 且检索到的内容会高亮
    if grep != "" {
        if strings.Contains(messageStr, grep) {
            messageStr = strings.ReplaceAll(messageStr, grep, WrapColor(grep, Yellow))
            log.Print("[", WrapColor(source.Deployment, Green), "] [", WrapColor(source.Namespace, Cyan), "] ", messageStr)
        }
    } else {
        log.Print("[", WrapColor(source.Deployment, Green), "] [", WrapColor(source.Namespace, Cyan), "] ", messageStr)
    }
    return true
}

var PROJECT_ID_MAP = map[string]string{
    "weike": "1",
    "dayou": "40",
    "oc": "41",
}

func main() {
    log.SetFlags(0)
    conf := initConf()
    sources := getAllSources(conf)

    addEnvFlag := flag.Bool("a", false, "新增项目")
    deployment := flag.String("d", "", "项目名")
    debug := flag.Bool("debug", false, "DEBUG: 输出 WSS 路径但不进行连接")
    env := flag.String("e", "dev", "集群选择: dev | prod | prod-tokyo")
    tailLines := flag.String("l", "500", "tail行数: 500 | 1000 | 2000")
    name := flag.String("n", "", "服务名")
    namespace := flag.String("ns", "", "命名空间：如 dev1")
    refreshTokenFlag := flag.Bool("r", false, "刷新 token")
    source := flag.String("s", "", fmt.Sprintf(`日志来源，即配置文件中的别名/Source of source in $HOME/.kkconfig.yaml %v`, sources))
    _type := flag.String("t", "api", "服务类型: api | script")
    project := flag.String("p", "weike", "项目区分: weike | dayou | oc")
    grep := flag.String("g", "", "日志内容检索")

    flag.Parse()
    if len(os.Args) < 2 {
        flag.Usage()
        os.Exit(0)
    }

    if *addEnvFlag {
        addSource()
        os.Exit(0)
    }

    if *refreshTokenFlag {
        refreshToken()
        os.Exit(0)
    }

    if conf.User.Token == "" {
        log.Fatal("未找到有效的 token，请先登录效能平台 https://value.weike.fm 再执行 kklog -r 注入 token")
        os.Exit(1)
    }

    // 监听主动退出信号
    interrupt := make(chan os.Signal, 1)
    signal.Notify(interrupt, os.Interrupt)

    // 用于标记某事做完的常用做法 空结构体管道
    done := make(chan struct{})

    // 计时器 定时回复消息 ping wss server
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    var curSource *Source
    var ok bool
    if len(*source) > 0 {
        // 传入日志源
        curSource, ok = conf.EnvMap[*source]
        if !ok {
            log.Printf(`日志来源[ %v ]未定义，请检查\n`, *source)
            os.Exit(0)
        }
    } else {
        // 未传日志源
        curSource = &Source{}
    }

    if *namespace != "" {
        curSource.Namespace = *namespace
    }
    if *deployment != "" {
        curSource.Deployment = *deployment
    }
    if *name != "" {
        curSource.Name = *name
    }
    if curSource.Type == "" {
        curSource.Type = *_type
    }

    if curSource.Project == "" {
        log.Printf(`[%v] 还未配置所属项目，请更新配置文件或用 -p 指定，如已指定请忽略。\n`, curSource.Source)
    }

    if *project != "" {
        curSource.Project = *project
    }

    projectId, getPidOK := PROJECT_ID_MAP[curSource.Project]
    if (!getPidOK) {
        log.Printf(`项目[ %v ]不存在，请检查\n`, curSource.Project)
        os.Exit(1)
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
        "namespace=" + curSource.Namespace,
        "label=app=" + curSource.Deployment + ",cicd_env=stable,name=" + curSource.Name + ",type=" + curSource.Type + ",version=stable",
    }

    var link = `wss://value.weike.fm/ws/api/k8s/` + *env + `/pods/log`
    link += "?" + strings.Join(args, "&")
    log.Printf("Connecting:[%s]\nNamespace:[%s]\nLink:[%s]", WrapColor(curSource.Name, Green), WrapColor(curSource.Namespace, Cyan), WrapColor(link, Blue))
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
            r := handleMessage(c, curSource, *grep)
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
