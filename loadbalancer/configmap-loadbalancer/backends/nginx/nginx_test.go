package nginx

import (
	"reflect"
	"testing"
	
	"k8s.io/contrib/loadbalancer/configmap-loadbalancer/backends"
)

func TestGetNGINXConfigFileName(t *testing.T) {
	path := "path"
	name := "filename"
	expected := "path/filename.conf"
	if result := getNGINXConfigFileName(path, name); result != expected {
		t.Errorf("getNGINXConfigFileName(%q, %q) returned %q. Expected %q. ", path, name, result, expected)
	}
}

func TestGenerateNGINXCfg(t *testing.T) {
	certPath := "/etc/nginx/ssl"
	name := "nginxApp"
	configObject := generateExampleBackendObject()
	
	expectedNginxConfig := NGINXConfig {
		Upstream: Upstream{
			Name: "nginxApp-localhost-helloApp",
			UpstreamServer: UpstreamServer{
				Address: "helloApp", 
				Port: "8080",
			},
		}, 
		Server: Server{
			Name: "localhost",
			BindIP: "127.0.0.1",
			BindPort: "80",
			Location: Location{
				Path: "/hello",
				Upstream: Upstream{
					Name: "nginxApp-localhost-helloApp",
					UpstreamServer: UpstreamServer{
						Address: "helloApp", 
						Port: "8080",
					},
				},
			},
		},
	}
	
	nginxConfig := generateNGINXCfg(certPath, name, configObject)
	if !reflect.DeepEqual(nginxConfig, expectedNginxConfig) {
		t.Error(
			"In generateNGINXCfg()",
			"expected", expectedNginxConfig,
			"got", nginxConfig,
		)
	}	
}

func generateExampleBackendObject() backends.BackendConfig{
	backendConfig := backends.BackendConfig{
		Host: "localhost",
		Namespace: "default",
		BindIp: "127.0.0.1",
		BindPort: 80,
		TargetServiceName: "helloApp",
		TargetServiceId: "123",
		TargetPort: 8080,
		SSL: false,
		Path: "/hello",
	}
	return backendConfig
}