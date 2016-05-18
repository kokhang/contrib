package nginx

import (
	"bytes"
	"html/template"
	"os"
	"os/exec"
	"path"

	"github.com/golang/glog"

	"k8s.io/contrib/loadbalancer/backends"
)

// NGINXController Updates NGINX configuration, starts and reloads NGINX
type NGINXController struct {
	nginxConfdPath string
	nginxCertsPath string
}

type NGINXConfig struct {
	upstream Upstream
	server  Server
}

// Upstream describes an NGINX upstream
type Upstream struct {
	name            string
	upstreamServer UpstreamServer
}

// UpstreamServer describes a server in an NGINX upstream
type UpstreamServer struct {
	address string
	port    string
}

// Server describes an NGINX server
type Server struct {
	name              string
	location          Location
	ssl               bool
	sslCertificate    string
	sslCertificateKey string
}

// Location describes an NGINX location
type Location struct {
	path     string
	upstream Upstream
}

// NewNGINXController creates a NGINX controller
func NewNGINXController(conf map[string]string) (backends.BackendController, error) {

	ngxc := NGINXController{
		nginxConfdPath: path.Join(conf["CONFIG_PATH"], "conf.d"),
		nginxCertsPath: path.Join(conf["CONFIG_PATH"], "ssl"),
	}

	createCertsDir()

	return &ngxc, nil
}

func (nginxC *NGINXController) Name() string {
    return "NGINXController"
}

// DeleteIngress deletes the configuration file, which corresponds for the
// specified ingress from NGINX conf directory
func (nginx *NGINXController) DeleteIngress(name string) {
	filename := getNGINXConfigFileName(name)
	glog.V(3).Infof("deleting %v", filename)

	if err := os.Remove(filename); err != nil {
		glog.Warningf("Failed to delete %v: %v", filename, err)
	}
}

// AddConfig creates or updates a file with
// the specified configuration for the specified config
func (nginx *NGINXController) AddConfig(name string, config backends.BackendConfig) {
	glog.Infof("Updating NGINX configuration")
	glog.Infof("Received config %s: %v", name, config)
	nginxConfig := generateNGINXCfg(nginx.nginxConfdPath, name, config)
	filename := path.Join(nginx.nginxConfdPath, name+".conf")
	nginx.templateIt(nginxConfig, filename)
}

func getNGINXConfigFileName(name string) string {
	return path.Join(nginx.nginxConfdPath, name+".conf")
}

func templateIt(config NGINXConfig, filename string) {
	tmpl, err := template.New("nginx.tmpl").ParseFiles("nginx.tmpl")
	if err != nil {
		glog.Fatal("Failed to parse template file")
	}

	glog.Infof("Writing NGINX conf to %v", filename)

	if glog {
		tmpl.Execute(os.Stdout, config)
	}

	w, err := os.Create(filename)
	if err != nil {
		glog.Fatalf("Failed to open %v: %v", filename, err)
	}
	defer w.Close()

	if err := tmpl.Execute(w, config); err != nil {
		glog.Fatalf("Failed to write template %v", err)
	}

	glog.Infof("NGINX configuration file had been updated")
}

// Reload reloads NGINX
func reload() {
	shellOut("nginx -s reload")
}

// Start starts NGINX
func start() {
	shellOut("nginx")
}

func createCertsDir(path string) {
	if err := os.Mkdir(path, os.ModeDir); err != nil {
		if os.IsExist(err) {
			glog.Infof("%v already exists", err)
			return
		}
		glog.Fatalf("Couldn't create directory %v: %v", path, err)
	}
}

func generateNGINXCfg(path string, name string, config backends.BackendConfig) NGINXConfig {

	upsName := getNameForUpstream(name, config.Host, config.TargetServiceName)
	upstream := createUpstream(name, config)

	serverName := config.Host
	server := Server{name: serverName}

	if config.SSL {
		pemFile := addOrUpdateCertAndKey(path, name, config.TlsCert, config.TlsKey)
		server.ssl = true
		server.sslCertificate = pemFile
		server.sslCertificateKey = pemFile
	}

	loc := Location{
		path: config.Path,
		upstream: upstream,
	}
	server.location = loc

	return NGINXConfig {
		upstream: upstream, 
		server: server,
	}
}

func createUpstream(name string, backend backends.BackendConfig) Upstream {
	ups := Upstream{
		Name: name,
		UpstreamServer: UpstreamServer{
			UpstreamServer{
				address: backend.TargetServiceName, 
				port: backend.TargetPort,
			},
		},
	}
	return ups
}

func addOrUpdateCertAndKey(path string, name string, cert string, key string) string {
	pemFileName := path + "/" + name + ".pem"

	pem, err := os.Create(pemFileName)
	if err != nil {
		glog.Fatalf("Couldn't create pem file %v: %v", pemFileName, err)
	}
	defer pem.Close()

	_, err = pem.WriteString(key)
	if err != nil {
		glog.Fatalf("Couldn't write to pem file %v: %v", pemFileName, err)
	}

	_, err = pem.WriteString("\n")
	if err != nil {
		glog.Fatalf("Couldn't write to pem file %v: %v", pemFileName, err)
	}

	_, err = pem.WriteString(cert)
	if err != nil {
		glog.Fatalf("Couldn't write to pem file %v: %v", pemFileName, err)
	}

	return pemFileName
}

func getNameForUpstream(name string, host string, service string) string {
	return fmt.Sprintf("%v-%v-%v", Name, host, service)
}

func shellOut(cmd string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	glog.Infof("executing %s", cmd)

	command := exec.Command("sh", "-c", cmd)
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Start()
	if err != nil {
		glog.Fatalf("Failed to execute %v, err: %v", cmd, err)
	}

	err = command.Wait()
	if err != nil {
		glog.Errorf("Command %v stdout: %q", cmd, stdout.String())
		glog.Errorf("Command %v stderr: %q", cmd, stderr.String())
		glog.Fatalf("Command %v finished with error: %v", cmd, err)
	}
}
