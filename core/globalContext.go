package core

import (
	"errors"
	"flag"
	"fmt"
	"reflect"
	"strings"

	cfg "github.com/weibocom/motan-go/config"
)

const (
	// all default sections
	registrysSection     = "motan-registry"
	basicRefersSection   = "motan-basicRefer"
	refersSection        = "motan-refer"
	basicServicesSection = "motan-basicService"
	servicesSection      = "motan-service"
	agentSection         = "motan-agent"
	httpAgentSection     = "motan-http-agent"
	clientSection        = "motan-client"
	serverSection        = "motan-server"
	importSection        = "import-refer"
	dynamicSection       = "dynamic-param"
	SwitcherSection      = "switcher"

	// URLConfKey is config id
	// config Keys
	URLConfKey      = "conf-id"
	basicReferKey   = "basicRefer"
	basicServiceKey = "basicService"

	// default config file and path
	configFile = "./motan.yaml"
	configPath = "./"
	fileSuffix = ".yaml"

	// const for application pool
	basicConfig       = "basic.yaml"
	servicePath       = "services/"
	applicationPath   = "applications/"
	poolPath          = "pools/"
	poolNameSeparator = "-"
)

// Context for agent, client, server. context is created depends on  config file
type Context struct {
	ConfigFile       string
	Config           *cfg.Config
	RegistryURLs     map[string]*URL
	RefersURLs       map[string]*URL
	BasicReferURLs   map[string]*URL
	HttpRefersURLs   map[string]map[string]*URL
	ServiceURLs      map[string]*URL
	BasicServiceURLs map[string]*URL
	AgentURL         *URL
	ClientURL        *URL
	ServerURL        *URL
}

var (
	urlFields = map[string]bool{"protocol": true, "host": true, "port": true, "path": true, "group": true}
)

// all env flag in motan-go
var (
	Port         = flag.Int("port", 0, "agent listen port")
	Eport        = flag.Int("eport", 0, "agent export service port when as a reverse proxy server")
	Mport        = flag.Int("mport", 0, "agent manage port")
	HttpPort     = flag.Int("httpPort", 0, "http agent port")
	Pidfile      = flag.String("pidfile", "", "agent manage port")
	CfgFile      = flag.String("c", "", "motan run conf")
	LocalIP      = flag.String("localIP", "", "local ip for motan register")
	IDC          = flag.String("idc", "", "the idc info for agent or client.")
	DynamicConfs = flag.String("dynamicConf", "", "dynamic config file for config placeholder")
	Pool         = flag.String("pool", "", "application pool config. like 'application-idc-level'")
	Application  = flag.String("application", "", "assist for application pool config.")
	Recover      = flag.Bool("recover", false, "recover from accidental exit")
)

func (c *Context) confToURLs(section string) map[string]*URL {
	urls := map[string]*URL{}
	sectionConf, _ := c.Config.GetSection(section)
	for key, info := range sectionConf {
		url := confToURL(info.(map[interface{}]interface{}))
		url.Parameters[URLConfKey] = InterfaceToString(key)
		urls[key.(string)] = url
	}
	return urls
}

func confToURL(urlInfo map[interface{}]interface{}) *URL {
	urlParams := make(map[string]string)
	url := &URL{Parameters: urlParams}
	for key, value := range urlInfo {
		if _, ok := urlFields[key.(string)]; ok {
			if reflect.TypeOf(value) == nil {
				if key == "port" {
					value = "0"
				} else {
					value = ""
				}
			}
			switch key.(string) {
			case "protocol":
				url.Protocol = value.(string)
			case "host":
				url.Host = value.(string)
			case "port":
				url.Port = value.(int)
			case "path":
				url.Path = value.(string)
			case "group":
				url.Group = value.(string)
			}
		} else {
			urlParams[key.(string)] = InterfaceToString(value)
		}
	}
	url.Parameters = urlParams
	return url
}

func (c *Context) Initialize() {
	c.RegistryURLs = make(map[string]*URL)
	c.RefersURLs = make(map[string]*URL)
	c.BasicReferURLs = make(map[string]*URL)
	c.HttpRefersURLs = make(map[string]map[string]*URL)
	c.ServiceURLs = make(map[string]*URL)
	c.BasicServiceURLs = make(map[string]*URL)
	if c.ConfigFile == "" { // use flag as default config file name
		c.ConfigFile = *CfgFile
	}
	var cfgRs *cfg.Config
	var err error
	if *Pool != "" { // parse application pool configs
		if c.ConfigFile == "" {
			c.ConfigFile = configPath
		}
		if !strings.HasSuffix(c.ConfigFile, "/") {
			c.ConfigFile = c.ConfigFile + "/"
		}
		if cfgRs, err = parsePool(c.ConfigFile, *Pool); err != nil {
			fmt.Printf("parse configs fail. err:%s\n", err.Error())
			return
		}
	} else { // parse single config file and dynamic file
		if c.ConfigFile == "" {
			c.ConfigFile = configFile
		}
		if cfgRs, err = cfg.NewConfigFromFile(c.ConfigFile); err != nil {
			fmt.Printf("parse config fail. err:%s\n", err.Error())
			return
		}
		var dynamicFile string
		if *DynamicConfs != "" {
			dynamicFile = *DynamicConfs
		}
		if dynamicFile == "" && *IDC != "" {
			suffix := *IDC + ".yaml"
			idx := strings.LastIndex(c.ConfigFile, "/")
			if idx > -1 {
				dynamicFile = c.ConfigFile[0:idx+1] + suffix
			} else {
				dynamicFile = suffix
			}
		}
		if dynamicFile != "" {
			dc, _ := cfg.NewConfigFromFile(dynamicFile)
			dconfs := make(map[string]interface{})
			for k, v := range dc.GetOriginMap() {
				if _, ok := v.(map[interface{}]interface{}); ok { // v must be a single value
					continue
				}
				if ks, ok := k.(string); ok {
					dconfs[ks] = v
				}
			}
			cfgRs.ReplacePlaceHolder(dconfs)
		}
	}

	c.Config = cfgRs
	c.parseRegistrys()
	c.parseBasicRefers()
	c.parseRefers()
	c.parserBasicServices()
	c.parseServices()
	c.parseHostURL()
}

// pool config priority ： pool > application > service > basic
func parsePool(path string, pool string) (*cfg.Config, error) {
	c := cfg.NewConfig()
	var tempcfg *cfg.Config
	var err error

	// basic config
	tempcfg, err = cfg.NewConfigFromFile(path + basicConfig)
	if err == nil && tempcfg != nil {
		c.Merge(tempcfg)
	}

	// application
	poolPart := strings.Split(pool, poolNameSeparator)
	var appconfig *cfg.Config
	application := *Application
	if application == "" && len(poolPart) > 0 { // the first part be the application name
		application = poolPart[0]
	}
	application = path + applicationPath + application + fileSuffix
	if application != "" {
		appconfig, err = cfg.NewConfigFromFile(application)
		if err == nil && appconfig != nil {
			// import-refer
			is, err := appconfig.DIY(importSection)
			if err == nil && is != nil {
				if li, ok := is.([]interface{}); ok {
					for _, r := range li {
						tempcfg, err = cfg.NewConfigFromFile(path + servicePath + r.(string) + fileSuffix)
						if err == nil && tempcfg != nil {
							c.Merge(tempcfg)
						}
					}

				}
			}
			c.Merge(appconfig) // application config > service default config
		}
	}

	// pool
	if len(poolPart) > 0 { // step loading
		base := ""
		for _, v := range poolPart {
			base = base + v
			tempcfg, err = cfg.NewConfigFromFile(path + poolPath + base + fileSuffix)
			if err == nil && tempcfg != nil {
				c.Merge(tempcfg)
			}
			base = base + poolNameSeparator
		}
	}

	// replace dynamic param
	dp, err := c.GetSection(dynamicSection)
	if err == nil && len(dp) > 0 {
		ph := make(map[string]interface{}, len(dp))
		for k, v := range dp {
			if sk, ok := k.(string); ok {
				ph[sk] = v
			}
		}
		c.ReplacePlaceHolder(ph)
	}

	if len(c.GetOriginMap()) == 0 {
		return nil, errors.New("parse " + pool + " pool fail.")
	}
	return c, nil

}

// parse host url including agenturl, clienturl, serverurl
func (c *Context) parseHostURL() {
	agentInfo, _ := c.Config.GetSection(agentSection)
	c.AgentURL = confToURL(agentInfo)
	clientInfo, _ := c.Config.GetSection(clientSection)
	c.ClientURL = confToURL(clientInfo)
	serverInfo, _ := c.Config.GetSection(serverSection)
	c.ServerURL = confToURL(serverInfo)
}

func (c *Context) parseRegistrys() {
	c.RegistryURLs = c.confToURLs(registrysSection)
}

func (c *Context) basicConfToURLs(section string) map[string]*URL {
	newURLs := map[string]*URL{}
	urls := c.confToURLs(section)
	var basicURLs map[string]*URL
	var basicKey string
	if section == servicesSection {
		basicURLs = c.BasicServiceURLs
		basicKey = basicServiceKey
	} else if section == refersSection {
		basicURLs = c.BasicReferURLs
		basicKey = basicReferKey
	}
	for key, url := range urls {
		var newURL *URL
		if basicConfName := url.GetParam(basicKey, ""); basicConfName != "" {
			if basicURL, ok := basicURLs[basicConfName]; ok {
				newURL = basicURL.Copy()
				if url.Protocol != "" {
					newURL.Protocol = url.Protocol
				}
				if url.Host != "" {
					newURL.Host = url.Host
				}
				if url.Port != 0 {
					newURL.Port = url.Port
				}
				if url.Group != "" {
					newURL.Group = url.Group
				}
				if url.Path != "" {
					newURL.Path = url.Path
				}
				newURL.MergeParams(url.Parameters)
				fmt.Printf("load %s configuration success. url: %s\n", basicConfName, newURL.GetIdentity())
			} else {
				newURL = url
				fmt.Printf("can not found %s: %s. url: %s\n", basicKey, basicConfName, url.GetIdentity())
			}
		} else {
			newURL = url
		}
		newURLs[key] = newURL
	}
	return newURLs
}

func (c *Context) agentConfToURLs(section string, subSection string) map[string]*URL {
	urls := map[string]*URL{}
	sectionConf, _ := c.Config.GetSection(section)
	idc := sectionConf["idc"]
	domains := sectionConf["domains"]
	// todo
	upstream := sectionConf["upstream"]
	if idc != nil && domains != nil && upstream != nil {
		for _, domain := range strings.Split(domains.(string), ",") {
			urlInfo := sectionConf[subSection].(map[interface{}]interface{})
			urlInfo["path"] = upstream
			urlInfo["group"] = idc.(string) + "-" + domain
			url := confToURL(urlInfo)
			urls[domain] = url
		}
	}
	return urls
}

func (c *Context) agentConfToHttpURLs(section string, subSection string) map[string]map[string]*URL {
	urls := map[string]map[string]*URL{}
	sectionConf, _ := c.Config.GetSection(section)
	idc := sectionConf["idc"]
	domains := sectionConf["domains"]
	upstream := sectionConf["upstream"]
	location := sectionConf["location"].(string)
	if idc != nil && domains != nil && upstream != nil {
		for _, domain := range strings.Split(domains.(string), ",") {
			urlInfo := sectionConf[subSection].(map[interface{}]interface{})
			urlInfo["path"] = upstream
			urlInfo["group"] = idc.(string) + "-" + domain
			url := confToURL(urlInfo)
			value, ok := urls[domain]
			if ok {
				value[location] = url
			} else {
				urls[domain] = make(map[string]*URL)
			}
			urls[domain][location] = url
		}
	}
	return urls
}

func (c *Context) parseRefers() {
	c.RefersURLs = c.basicConfToURLs(refersSection)

	for k, v := range c.agentConfToHttpURLs(httpAgentSection, "agent-referer") {
		c.HttpRefersURLs[k] = v
	}
}

func (c *Context) parseBasicRefers() {
	c.BasicReferURLs = c.confToURLs(basicRefersSection)
}

func (c *Context) parseServices() {
	c.ServiceURLs = c.basicConfToURLs(servicesSection)

	for k, v := range c.agentConfToURLs(httpAgentSection, "agent-service") {
		c.ServiceURLs[k] = v
	}
}

func (c *Context) parserBasicServices() {
	c.BasicServiceURLs = c.confToURLs(basicServicesSection)
}
