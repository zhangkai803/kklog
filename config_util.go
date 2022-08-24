package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"reflect"
	"runtime"
)

const ConfigPath = "/.kkconf.yaml"
const TokenRefreshUrl = "https://value.weike.fm/login"

type User struct {
	Name  string `yaml:"name"`
	Token string `yaml:"token"`
}

type Env struct {
	Source     string `yaml:"source"`
	Deployment string `yaml:"deployment"`
	Name       string `yaml:"name"`
	Namespace  string `yaml:"namespace"`
	Type       string `yaml:"type"`
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
		fmt.Printf(`解析配置文件失败，请检查 $HOME/.kkconf.yaml 是否存在` + fileFormatTip)
		panic("yamlFile.Get err: " + err.Error())
	}

	c := Conf{}
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		fmt.Printf(`解析配置文件失败，请检查格式是否正确` + fileFormatTip)
		panic("yaml.Unmarshal err: " + err.Error())
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
		key := typeInfo.Field(i).Name
		var v string
		fmt.Printf(`%s:`, key)
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
	os.Exit(0)
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
	os.Exit(0)
}
