package models

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"io/ioutil"

	"github.com/astaxie/beego/httplib"
	"github.com/beego/beego/v2/core/logs"
	"github.com/buger/jsonparser"
)

func init() {
	//获取路径
	ExecPath, _ = filepath.Abs(filepath.Dir(os.Args[0]))
	Save = make(chan *JdCookie)
	go func() {
		for {
			ss := <-Save
			if V4Config != "" {
				V4Handle(ss)
			} else {
				QLHandle(ss)
			}

		}
	}()
}

type JdCookie struct {
	Priority  int
	ScanedAt  int
	PtKey     string
	PtPin     string
	Note      string
	Available string
	Nickname  string
	BeanNum   string
}

var True = "true"
var False = "false"

var Save chan *JdCookie

var ExecPath string

var Token = ""
var QlAddress = ""
var QlUserName = ""
var QlPassword = ""
var V4Config = ""

func GetToken() error {
	req := httplib.Post(QlAddress + "/api/login")
	req.Header("Content-Type", "application/json;charset=UTF-8")
	req.Body(fmt.Sprintf(`{"username":"%s","password":"%s"}`, QlUserName, QlPassword))
	if rsp, err := req.Response(); err == nil {
		data, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			return err
		}
		Token, _ = jsonparser.GetString(data, "token")
	}
	return nil
}

const (
	GET  = "GET"
	POST = "POST"
	PUT  = "PUT"
)

func V4Handle(ck *JdCookie) error {
	config := ""
	f, err := os.OpenFile(V4Config, os.O_RDWR|os.O_CREATE, 0777) //打开文件 |os.O_RDWR
	if err != nil {
		return err
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadString('\n') //以'\n'为结束符读入一行
		if err != nil || io.EOF == err {
			break
		}
		if pt := regexp.MustCompile(`^#?\s?Cookie(\d+)=\S+pt_key=(.*);pt_pin=([^'";\s]+);?`).FindStringSubmatch(line); len(pt) != 0 {
			if nck := GetJdCookie(pt[3]); nck == nil {
				SaveJdCookie(JdCookie{
					PtKey:     pt[2],
					PtPin:     pt[3],
					Available: True,
				})
			}
			continue
		}
		if strings.Contains(line, "TempBlockCookie") {
			continue
		}
		config += line
	}
	TempBlockCookie := ""
	for i, ck := range GetJdCookies() {
		if ck.Available == False {
			TempBlockCookie += fmt.Sprintf("%d ", i+1)
		}
		config = fmt.Sprintf("Cookie%d=\"pt_key=%s;pt_pin=%s;\"\n", i+1, ck.PtKey, ck.PtPin) + config
	}
	config = fmt.Sprintf(`TempBlockCookie="%s"`, TempBlockCookie) + "\n" + config
	f.Truncate(0)
	f.Seek(0, 0)
	if _, err := io.WriteString(f, config); err != nil {
		return err
	}
	return nil
}

func QLHandle(ck *JdCookie) error {
	if Token == "" {
		GetToken()
	}
	var data = request("/api/envs?searchValue=JD_COOKIE")
	value, _ := jsonparser.GetString(data, "data", "[0]", "value")
	_id, _ := jsonparser.GetString(data, "data", "[0]", "_id")
	if _id == "" {
		request("/api/envs", POST, `{"name":"JD_COOKIE","value":"pt_key=`+ck.PtKey+`;pt_pin=`+ck.PtPin+`;"}`)
	}
	newValue := ""
	for _, pt := range regexp.MustCompile(`pt_key=(\S+);pt_pin=([^;\s]+);?`).FindAllStringSubmatch(value, -1) {
		if len(pt) == 3 {
			if nck := GetJdCookie(pt[2]); nck == nil {
				SaveJdCookie(JdCookie{
					PtKey:     pt[1],
					PtPin:     pt[2],
					Available: True,
				})
			}
		}
	}
	for _, ck := range GetJdCookies() {
		if ck.Available == True {
			newValue += fmt.Sprintf("pt_key=%s;pt_pin=%s;\\n", ck.PtKey, ck.PtPin)
		}
	}
	request("/api/envs", PUT, `{"name":"JD_COOKIE","value":"`+newValue+`","_id":"`+_id+`"}`)
	return nil
}

func request(ss ...string) []byte {
	var api, method, body string
	for _, s := range ss {
		if s == GET || s == POST || s == PUT {
			method = s
		} else if strings.Contains(s, "api") {
			api = s
		} else {
			body = s
		}
	}
	var req *httplib.BeegoHTTPRequest
	for {
		if method == POST {
			req = httplib.Post(QlAddress + api)
		} else if method == PUT {
			req = httplib.Put(QlAddress + api)
		} else {
			req = httplib.Get(QlAddress + api)
		}
		req.Header("Authorization", "Bearer "+Token)
		if body != "" {
			req.Header("Content-Type", "application/json;charset=UTF-8")
			req.Body(body)
		}

		if data, err := req.Bytes(); err == nil {
			code, _ := jsonparser.GetInt(data, "code")
			if code == 200 {
				return data
			} else {
				logs.Warn(string(data))
				GetToken()
			}
		}
	}
	return []byte{}
}
