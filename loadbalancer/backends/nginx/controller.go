/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nginx

import (
	"reflect"
	"time"

	"github.com/golang/glog"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/controller/framework"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/watch"
)

// NginxController watches Kubernetes API for ConfigMape changes
// and reconfigures NGINX when needed
type NginxController struct {
	client         *client.Client
	configMapController  *framework.Controller
	configMapLister      StoreToConfigMapLister
	configMapQueue       *taskQueue
	stopCh         chan struct{}
//	nginx          *nginx.NGINXController
}

const (
	emptyHost = ""
)

var keyFunc = framework.DeletionHandlingMetaNamespaceKeyFunc

// NewNginxController creates a controller
func NewNginxController(kubeClient *client.Client, resyncPeriod time.Duration, namespace string) (*NginxController, error) {
	nginxController := NginxController{
		client: kubeClient,
		stopCh: make(chan struct{}),
	}

	nginxController.configMapQueue = NewTaskQueue(nginxController.syncConfigMap)

	configMapHandlers := framework.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			addConfigMap := obj.(*api.ConfigMap)
			glog.Infof("Adding ConfigMap: %v", addConfigMap.Name)
			nginxController.configMapQueue.enqueue(obj)
		},
		DeleteFunc: func(obj interface{}) {
			remConfigMap := obj.(*api.ConfigMap)
			glog.Infof("Removing ConfigMap: %v", remConfigMap.Name)
			nginxController.configMapQueue.enqueue(obj)
		},
		UpdateFunc: func(old, cur interface{}) {
			if !reflect.DeepEqual(old, cur) {
				glog.Infof("ConfigMap %v changed, syncing",
					cur.(*api.ConfigMap).Name)
				nginxController.configMapQueue.enqueue(cur)
			}
		},
	}
	nginxController.configMapLister.Store, nginxController.configMapController = framework.NewInformer(
		&cache.ListWatch{
			ListFunc:  configMapListFunc(kubeClient, namespace),
			WatchFunc: configMapWatchFunc(kubeClient, namespace),
		},
		&api.ConfigMap{}, resyncPeriod, configMapHandlers)
   
    return &nginxController, nil
}

// Run starts the loadbalancer controller
func (nginxController *NginxController) Run() {
	go nginxController.configMapController.Run(nginxController.stopCh)
    go nginxController.configMapQueue.run(time.Second, nginxController.stopCh)
	<-nginxController.stopCh
}

func configMapListFunc(c *client.Client, ns string) func(api.ListOptions) (runtime.Object, error) {
	return func(opts api.ListOptions) (runtime.Object, error) {
		return c.ConfigMaps(ns).List(opts)
	}
}

func configMapWatchFunc(c *client.Client, ns string) func(options api.ListOptions) (watch.Interface, error) {
	return func(options api.ListOptions) (watch.Interface, error) {
		return c.ConfigMaps(ns).Watch(options)
	}
}

func (nginxController *NginxController) syncConfigMap(key string) {
	glog.Infof("Syncing %v", key)

	obj, configMapExists, err := nginxController.configMapLister.Store.GetByKey(key)
	if err != nil {
		nginxController.configMapQueue.requeue(key, err)
		return
	}
    
    if !configMapExists {
		glog.Infof("Deleting ConfigMap: %v\n", key)
    } else {
        configMap := obj.(*api.ConfigMap)
        glog.Infof("Config Map Name: %v. Data: %v", configMap.Name, configMap.Data)
    }   

	// // defaut/some-configmap -> default-some-configmap
	// name := strings.Replace(key, "/", "-", -1)

	// if !configMapExists {
	// 	glog.V(2).Infof("Deleting Ingress: %v\n", key)
	// 	lbc.nginx.DeleteIngress(name)
	// } else {
	// 	glog.V(2).Infof("Adding or Updating Ingress: %v\n", key)

	// 	ing := obj.(*extensions.Ingress)

	// 	pems := lbc.updateCertificates(ing)

	// 	nginxCfg := lbc.generateNGINXCfg(ing, pems)
	// 	lbc.nginx.AddOrUpdateIngress(name, nginxCfg)
	// }

	// lbc.nginx.Reload()
}

/*
func (lbc *LoadBalancerController) updateCertificates(ing *extensions.Ingress) map[string]string {
	pems := make(map[string]string)

	for _, tls := range ing.Spec.TLS {
		secretName := tls.SecretName
		secret, err := lbc.client.Secrets(ing.Namespace).Get(secretName)
		if err != nil {
			glog.Warningf("Error retriveing secret %v for ing %v: %v", secretName, ing.Name, err)
			continue
		}
		cert, ok := secret.Data[api.TLSCertKey]
		if !ok {
			glog.Warningf("Secret %v has no private key", secretName)
			continue
		}
		key, ok := secret.Data[api.TLSPrivateKeyKey]
		if !ok {
			glog.Warningf("Secret %v has no cert", secretName)
			continue
		}

		pemFileName := lbc.nginx.AddOrUpdateCertAndKey(secretName, string(cert), string(key))

		for _, host := range tls.Hosts {
			pems[host] = pemFileName
		}
		if len(tls.Hosts) == 0 {
			pems[emptyHost] = pemFileName
		}
	}

	return pems
}

func (lbc *LoadBalancerController) generateNGINXCfg(ing *extensions.Ingress, pems map[string]string) nginx.IngressNGINXConfig {
	upstreams := make(map[string]nginx.Upstream)

	if ing.Spec.Backend != nil {
		name := getNameForUpstream(ing, emptyHost, ing.Spec.Backend.ServiceName)
		upstream := lbc.createUpstream(name, ing.Spec.Backend, ing.Namespace)
		upstreams[name] = upstream
	}

	var servers []nginx.Server

	for _, rule := range ing.Spec.Rules {
		if rule.IngressRuleValue.HTTP == nil {
			continue
		}

		serverName := rule.Host

		if rule.Host == emptyHost {
			glog.Warningf("Host field of ingress rule in %v/%v is empty", ing.Namespace, ing.Name)
		}

		server := nginx.Server{Name: serverName}

		if pemFile, ok := pems[serverName]; ok {
			server.SSL = true
			server.SSLCertificate = pemFile
			server.SSLCertificateKey = pemFile
		}

		var locations []nginx.Location
		rootLocation := false

		for _, path := range rule.HTTP.Paths {
			upsName := getNameForUpstream(ing, rule.Host, path.Backend.ServiceName)

			if _, exists := upstreams[upsName]; !exists {
				upstream := lbc.createUpstream(upsName, &path.Backend, ing.Namespace)
				upstreams[upsName] = upstream
			}
			loc := nginx.Location{Path: pathOrDefault(path.Path)}

			loc.Upstream = upstreams[upsName]
			locations = append(locations, loc)

			if loc.Path == "/" {
				rootLocation = true
			}
		}

		if rootLocation == false && ing.Spec.Backend != nil {
			upsName := getNameForUpstream(ing, emptyHost, ing.Spec.Backend.ServiceName)
			loc := nginx.Location{Path: pathOrDefault("/")}
			loc.Upstream = upstreams[upsName]
			locations = append(locations, loc)
		}

		server.Locations = locations
		servers = append(servers, server)
	}

	if len(ing.Spec.Rules) == 0 && ing.Spec.Backend != nil {
		server := nginx.Server{Name: emptyHost}

		if pemFile, ok := pems[emptyHost]; ok {
			server.SSL = true
			server.SSLCertificate = pemFile
			server.SSLCertificateKey = pemFile
		}

		var locations []nginx.Location

		upsName := getNameForUpstream(ing, emptyHost, ing.Spec.Backend.ServiceName)

		loc := nginx.Location{Path: "/"}
		loc.Upstream = upstreams[upsName]
		locations = append(locations, loc)

		server.Locations = locations
		servers = append(servers, server)
	}

	return nginx.IngressNGINXConfig{Upstreams: upstreamMapToSlice(upstreams), Servers: servers}
}

func (lbc *LoadBalancerController) createUpstream(name string, backend *extensions.IngressBackend, namespace string) nginx.Upstream {
	ups := nginx.NewUpstreamWithDefaultServer(name)

	svcKey := namespace + "/" + backend.ServiceName
	svcObj, svcExists, err := lbc.svcLister.Store.GetByKey(svcKey)
	if err != nil {
		glog.V(3).Infof("error getting service %v from the cache: %v", svcKey, err)
	} else {
		if svcExists {
			svc := svcObj.(*api.Service)
			if svc.Spec.ClusterIP != "None" && svc.Spec.ClusterIP != "" {
				upsServer := nginx.UpstreamServer{Address: svc.Spec.ClusterIP, Port: backend.ServicePort.String()}
				ups.UpstreamServers = []nginx.UpstreamServer{upsServer}
			} else if svc.Spec.ClusterIP == "None" {
				endps, err := lbc.endpLister.GetServiceEndpoints(svc)
				if err != nil {
					glog.V(3).Infof("error getting endpoints for service %v from the cache: %v", svc, err)
				} else {
					upsServers := endpointsToUpstreamServers(endps, backend.ServicePort.IntValue())
					if len(upsServers) > 0 {
						ups.UpstreamServers = upsServers
					}
				}
			}
		}
	}

	return ups
}

func pathOrDefault(path string) string {
	if path == "" {
		return "/"
	} else {
		return path
	}
}

func endpointsToUpstreamServers(endps api.Endpoints, servicePort int) []nginx.UpstreamServer {
	var upsServers []nginx.UpstreamServer
	for _, subset := range endps.Subsets {
		for _, port := range subset.Ports {
			if port.Port == servicePort {
				for _, address := range subset.Addresses {
					ups := nginx.UpstreamServer{Address: address.IP, Port: fmt.Sprintf("%v", servicePort)}
					upsServers = append(upsServers, ups)
				}
				break
			}
		}
	}

	return upsServers
}

func getNameForUpstream(ing *extensions.Ingress, host string, service string) string {
	return fmt.Sprintf("%v-%v-%v-%v", ing.Namespace, ing.Name, host, service)
}

func upstreamMapToSlice(upstreams map[string]nginx.Upstream) []nginx.Upstream {
	result := make([]nginx.Upstream, 0, len(upstreams))

	for _, ups := range upstreams {
		result = append(result, ups)
	}

	return result
}
*/
