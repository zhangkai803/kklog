package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/pbkdf2"

	"gopkg.in/yaml.v3"
)

var (
	SALT       = "saltysalt"
	ITERATIONS = 1003
	KEYLENGTH  = 16
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
    DefaultSource string `yaml:"default_source"`
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

# 此处配置默认抓取的日志来源 为空默认为envs[0]
default_source: ""
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

# 此处配置默认抓取的日志来源 为空默认为envs[0]
default_source: ""
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
    if osName != "darwin" {
        fmt.Println("Not Supported.")
        os.Exit(-1)
    }
    db, err := sql.Open("sqlite3", "/Users/k/Library/Application Support/Google/Chrome/Profile 1/Cookies")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("select encrypted_value from cookies c where host_key = \"value.weike.fm\" and name = \"value_token\"")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
    var token string
    for rows.Next() {
		var encrypted_value []byte
		err = rows.Scan(&encrypted_value)
		if err != nil {
			log.Fatal(err)
		}
        iv := []byte{32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32}
        key, err := GetMasterKey()
        if err != nil {
            log.Fatal(err)
        }
        value, err := aes128CBCDecrypt(key, iv, encrypted_value[3:])
        token = string(value)
	}
    conf := GetConf()
    conf.User.Token = token
    marshal, err := yaml.Marshal(conf)
    handleError(err)
    err = os.WriteFile(getHome()+ConfigPath, marshal, 0777)
    handleError(err)
    fmt.Printf("Successfully refreshed token!\n")
}

/*
 * functions to get cookie and decrypt
 */

var (
	errWrongSecurityCommand   = errors.New("wrong security command")
	errCouldNotFindInKeychain = errors.New("could not be find in keychain")
)

func GetMasterKey() ([]byte, error) {
	var (
		cmd            *exec.Cmd
		stdout, stderr bytes.Buffer
	)
	// don't need chromium key file for macOS
	// defer os.Remove(item.TempChromiumKey)
	// Get the master key from the keychain
	// $ security find-generic-password -wa 'Chrome'
	cmd = exec.Command("security", "find-generic-password", "-wa", strings.TrimSpace("Chrome")) //nolint:gosec
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	if stderr.Len() > 0 {
		if strings.Contains(stderr.String(), "could not be found") {
			return nil, errCouldNotFindInKeychain
		}
		return nil, errors.New(stderr.String())
	}
	chromeSecret := bytes.TrimSpace(stdout.Bytes())
	if chromeSecret == nil {
		return nil, errWrongSecurityCommand
	}
	chromeSalt := []byte("saltysalt")
	// @https://source.chromium.org/chromium/chromium/src/+/master:components/os_crypt/os_crypt_mac.mm;l=157
	key := pbkdf2.Key(chromeSecret, chromeSalt, 1003, 16, sha1.New)
	if key == nil {
		return nil, errWrongSecurityCommand
	}
	return key, nil
}

func aes128CBCDecrypt(key, iv, encryptPass []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	encryptLen := len(encryptPass)
	if encryptLen < block.BlockSize() {
		return nil, errors.New("length of encrypted password less than block size")
	}

	dst := make([]byte, encryptLen)
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(dst, encryptPass)
	dst = pkcs5UnPadding(dst, block.BlockSize())
	return dst, nil
}

func pkcs5UnPadding(src []byte, blockSize int) []byte {
	n := len(src)
	paddingNum := int(src[n-1])
	if n < paddingNum || paddingNum > blockSize {
		return src
	}
	return src[:n-paddingNum]
}
