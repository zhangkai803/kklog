package main

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"

	"gopkg.in/yaml.v3"
)

const ConfigPath = "/.kkconf.yaml"
const TokenRefreshUrl = "https://value.weike.fm/login"

type User struct {
    Name  string `yaml:"name"`
    Token string `yaml:"token"`
}

type Env struct {
    Source     string `yaml:"source" desc:"自定义配置名"`
    Deployment string `yaml:"deployment" desc:"服务名称"`
    Name       string `yaml:"name" desc:"POD 名称"`
    Namespace  string `yaml:"namespace" desc:"命名空间"`
    Type       string `yaml:"type" desc:"服务类型[api/script]"`
}

type Conf struct {
    User   *User  `yaml:"user"`
    Envs   []*Env `yaml:"envs"`
    EnvMap map[string]*Env
}

var fileFormatTip string = `

配置文件格式：

user:
    name: 自定义
    token: 效能平台 token                   // 有效期 7 天，如果无法正常获取日志请尝试更换

envs:
    -
        source: wk_tag_manage               // 日志来源，自定义
        deployment: wk-tag-manage          // deployment 名
        name: wk-tag-manage                // pod 名
        type: api                          // api [服务] or script[脚本]
        namespace: dev1                    // 命名空间
    -
        source: tag-record-subscriber
        deployment: wk-tag-manage
        name: wk-tag-manage-tag-record-subscriber
        type: script
        namespace: dev1
`

var emptyConf string = `
user:
    # 用户名 非必须
    name: your.name
    # 效能平台 token 必须
    token: eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpZCI6ODksImV4cCI6MTY3Mzg0OTY5N30.M_a_wh0WgaX24XEBteKILW_W4Siaqoep5QZwWZdvt9Y

# 此处配置日志来源 注意列表格式
envs:
    -
        # 别名
        source: wtm_server
        # 服务名
        deployment: wk-tag-manage
        # pod 名
        name: wk-tag-manage
        # pod 类型
        type: api
        # 命名空间
        namespace: dev1
    -
        source: wtm_scriber
        deployment: wk-tag-manage
        name: wk-tag-manage-subscriber
        type: script
        namespace: dev1
`

func getHome() string {
    home := os.Getenv("HOME")
    if len(home) == 0 {
        panic("HOME is not set")
    }
    return home
}

func GetConf() *Conf {

    yamlFile, err := os.ReadFile(getHome() + ConfigPath)
    if err != nil {
        err = os.WriteFile(getHome() + ConfigPath, []byte(emptyConf), 0666)
        if err != nil {
            fmt.Println("配置文件创建失败，请检查目录权限或磁盘空间: ", getHome() + ConfigPath, err.Error())
        }
        fmt.Println("配置文件已初始化，请补充效能平台 token:", getHome() + ConfigPath)
        os.Exit(0)
    }

    c := Conf{}
    err = yaml.Unmarshal(yamlFile, &c)
    if err != nil {
        fmt.Println(`解析配置文件失败，请检查格式是否正确` , fileFormatTip , err.Error())
        os.Exit(1)
    }
    return &c

}

func initConf() *Conf {
    conf := GetConf()
    envMap := map[string]*Env{}
    for _, e := range conf.Envs {
        envMap[e.Source] = e
    }

    conf.EnvMap = envMap
    return conf
}

func handleError(err error) {
    if err != nil {
        fmt.Println(err)
        os.Exit(-1)
    }
}
func addEnv() {
    conf := GetConf()
    env := Env{}
    var typeInfo = reflect.TypeOf(env)
    num := typeInfo.NumField()
    s := reflect.ValueOf(&env).Elem() // 反射获取测试对象对应的struct枚举类型

    for i := 0; i < num; i++ {
        field := typeInfo.Field(i)
        var v string
        fmt.Printf(`%s %s:`, field.Name, field.Tag.Get("desc"))
        _, err := fmt.Scanln(&v)
        handleError(err)
        s.Field(i).SetString(v)
    }
    for i := range conf.Envs {
        if conf.Envs[i].Source == env.Source {
            fmt.Printf(`Source %s already exists!\n`, env.Source)
            os.Exit(-1)
        }
    }
    conf.Envs = append(conf.Envs, &env)
    marshal, err := yaml.Marshal(conf)
    handleError(err)
    err = os.WriteFile(getHome()+ConfigPath, marshal, 0777)
    handleError(err)
    fmt.Printf("Successfully added %s!\n", env.Name)
}

func refreshToken() {
    osName := runtime.GOOS
    var cmd *exec.Cmd
    if osName == "darwin" {
        cmd = exec.Command("sh", "-c", fmt.Sprintf(`open %s`, TokenRefreshUrl))
    } else if osName == "linux" {
        cmd = exec.Command("sh", "-c", fmt.Sprintf(`xdg-open %s`, TokenRefreshUrl))
    } else {
        fmt.Println("Not Supported.")
        os.Exit(-1)
    }
    err := cmd.Run()
    handleError(err)
    var token string
    fmt.Printf(`Please Enter Your Token:`)
    _, err = fmt.Scanln(&token)
    handleError(err)
    conf := GetConf()
    conf.User.Token = token
    marshal, err := yaml.Marshal(conf)
    handleError(err)
    err = os.WriteFile(getHome()+ConfigPath, marshal, 0777)
    handleError(err)
    fmt.Printf("Successfully refreshed token!\n")
}
