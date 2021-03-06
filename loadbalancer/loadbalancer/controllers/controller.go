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

package controllers

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"

	"k8s.io/contrib/loadbalancer/loadbalancer/backend"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/controller/framework"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/watch"
)

// ConfigMapController watches Kubernetes API for ConfigMap changes
// and reconfigures backend when needed
type LoadBalancerController struct {
	client              *client.Client
	configMapController *framework.Controller
	configMapLister     StoreToConfigMapLister
	svcController       *framework.Controller
	svcLister           cache.StoreToServiceLister
	nodeController      *framework.Controller
	nodeLister          cache.StoreToNodeLister
	configMapQueue      *taskQueue
	stopCh              chan struct{}
	backendController   backend.BackendController
}

// Values to verify the configmap object is a loadbalancer config
const (
	configLabelKey   = "app"
	configLabelValue = "loadbalancer"
)

var keyFunc = framework.DeletionHandlingMetaNamespaceKeyFunc

// NewLoadBalancerController creates a controller
func NewLoadBalancerController(kubeClient *client.Client, resyncPeriod time.Duration, namespace string, controller backend.BackendController) (*LoadBalancerController, error) {
	lbController := LoadBalancerController{
		client:            kubeClient,
		stopCh:            make(chan struct{}),
		backendController: controller,
	}
	lbController.configMapQueue = NewTaskQueue(lbController.syncConfigMap)

	configMapHandlers := framework.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			lbController.configMapQueue.enqueue(obj)
		},
		DeleteFunc: func(obj interface{}) {
			lbController.configMapQueue.enqueue(obj)
		},
		UpdateFunc: func(old, cur interface{}) {
			if !reflect.DeepEqual(old, cur) {
				lbController.configMapQueue.enqueue(cur)
			}
		},
	}
	lbController.configMapLister.Store, lbController.configMapController = framework.NewInformer(
		&cache.ListWatch{
			ListFunc:  configMapListFunc(kubeClient, namespace),
			WatchFunc: configMapWatchFunc(kubeClient, namespace),
		},
		&api.ConfigMap{}, resyncPeriod, configMapHandlers)

	svcHandlers := framework.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			addSvc := obj.(*api.Service)
			glog.Infof("Adding service: %v", addSvc.Name)
		},
		DeleteFunc: func(obj interface{}) {
			remSvc := obj.(*api.Service)
			glog.Infof("Removing service: %v", remSvc.Name)
		},
		UpdateFunc: func(old, cur interface{}) {
			if !reflect.DeepEqual(old, cur) {
				glog.Infof("Service %v changed, syncing",
					cur.(*api.Service).Name)
			}
		},
	}
	lbController.svcLister.Store, lbController.svcController = framework.NewInformer(
		&cache.ListWatch{
			ListFunc:  serviceListFunc(kubeClient, namespace),
			WatchFunc: serviceWatchFunc(kubeClient, namespace),
		},
		&api.Service{}, resyncPeriod, svcHandlers)

	nodeHandlers := framework.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			addNode := obj.(*api.Node)
			if nodeReady(*addNode) {
				configMapNodePortMap := lbController.getLBConfigMapNodePortMap()
				ip, err := getNodeHostIP(*addNode)
				if err != nil {
					glog.Errorf("Error getting IP for node %v", addNode.Name)
					return
				}
				go lbController.backendController.AddNodeHandler(*ip, configMapNodePortMap)
			}
		},
		DeleteFunc: func(obj interface{}) {
			delNode := obj.(*api.Node)
			if nodeReady(*delNode) {
				ip, _ := getNodeHostIP(*delNode)
				configMapNodePortMap := lbController.getLBConfigMapNodePortMap()
				go lbController.backendController.DeleteNodeHandler(*ip, configMapNodePortMap)
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			// Only sync nodes when they are in READY state and have their IPs changed
			curNode := cur.(*api.Node)
			if nodeReady(*curNode) {
				oldNode := old.(*api.Node)
				oldNodeIP, _ := getNodeHostIP(*oldNode)
				curNodeIP, _ := getNodeHostIP(*curNode)
				var configMapNodePortMap map[string]int
				if oldNodeIP == nil {
					glog.Infof("Updated node %v. IP set to %v. Syncing", curNode.Name, *curNodeIP)
					configMapNodePortMap = lbController.getLBConfigMapNodePortMap()
					go lbController.backendController.AddNodeHandler(*curNodeIP, configMapNodePortMap)
				} else if *oldNodeIP != *curNodeIP {
					glog.Infof("Updated node %v. IP changed from %v to %v. Syncing", curNode.Name, *oldNodeIP, *curNodeIP)
					configMapNodePortMap = lbController.getLBConfigMapNodePortMap()
					go lbController.backendController.UpdateNodeHandler(*oldNodeIP, *curNodeIP, configMapNodePortMap)
				}
			}
		},
	}

	lbController.nodeLister.Store, lbController.nodeController = framework.NewInformer(
		&cache.ListWatch{
			ListFunc: func(opts api.ListOptions) (runtime.Object, error) {
				return lbController.client.Get().
					Resource("nodes").
					FieldsSelectorParam(fields.Everything()).
					Do().
					Get()
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				return lbController.client.Get().
					Prefix("watch").
					Resource("nodes").
					FieldsSelectorParam(fields.Everything()).
					Param("resourceVersion", options.ResourceVersion).Watch()
			},
		},
		&api.Node{}, 0, nodeHandlers)

	return &lbController, nil
}

// Run starts the configmap controller
func (lbController *LoadBalancerController) Run() {
	go lbController.svcController.Run(lbController.stopCh)
	go lbController.nodeController.Run(lbController.stopCh)

	// Sleep for 3 seconds to give some times for service and node lister to be synced
	time.Sleep(time.Second * 3)
	go lbController.configMapController.Run(lbController.stopCh)
	go lbController.configMapQueue.run(time.Second, lbController.stopCh)
	<-lbController.stopCh
}

func configMapListFunc(c *client.Client, ns string) func(api.ListOptions) (runtime.Object, error) {
	return func(opts api.ListOptions) (runtime.Object, error) {
		opts.LabelSelector = labels.Set{configLabelKey: configLabelValue}.AsSelector()
		return c.ConfigMaps(ns).List(opts)
	}
}

func configMapWatchFunc(c *client.Client, ns string) func(options api.ListOptions) (watch.Interface, error) {
	return func(options api.ListOptions) (watch.Interface, error) {
		options.LabelSelector = labels.Set{configLabelKey: configLabelValue}.AsSelector()
		return c.ConfigMaps(ns).Watch(options)
	}
}

func serviceListFunc(c *client.Client, ns string) func(api.ListOptions) (runtime.Object, error) {
	return func(opts api.ListOptions) (runtime.Object, error) {
		return c.Services(ns).List(opts)
	}
}

func serviceWatchFunc(c *client.Client, ns string) func(options api.ListOptions) (watch.Interface, error) {
	return func(options api.ListOptions) (watch.Interface, error) {
		return c.Services(ns).Watch(options)
	}
}

func (lbController *LoadBalancerController) syncConfigMap(key string) {
	glog.Infof("Syncing configmap %v", key)

	// defaut/some-configmap -> default-some-configmap
	name := strings.Replace(key, "/", "-", -1)

	obj, configMapExists, err := lbController.configMapLister.Store.GetByKey(key)
	if err != nil {
		lbController.configMapQueue.requeue(key, err)
		return
	}

	if !configMapExists {
		go lbController.backendController.Delete(name)
	} else {
		configMap := obj.(*api.ConfigMap)
		configMapData := configMap.Data

		serviceName := configMapData["target-service-name"]
		namespace := configMapData["namespace"]
		serviceObj := lbController.getServiceObject(namespace, serviceName)
		// Check if the service exists
		if serviceObj == nil {
			return
		}

		bindPort, _ := strconv.Atoi(configMapData["bind-port"])
		targetServicePort, _ := strconv.Atoi(configMapData["target-port"])
		servicePort, err := getServicePort(serviceObj, targetServicePort)
		if err != nil {
			glog.Errorf("Error while getting the service port %v", err)
			return
		}

		nodes, _ := lbController.getReadyNodeIPs()
		backendConfig := backend.BackendConfig{
			Host:              configMapData["host"],
			Namespace:         namespace,
			TargetServiceName: serviceName,
			Protocol:          string(servicePort.Protocol),
			NodePort:          int(servicePort.NodePort),
			BindIPs:           nodes,
			BindPort:          bindPort,
			TargetPort:        targetServicePort,
		}
		go lbController.backendController.Create(name, backendConfig)
	}
}

func (lbController *LoadBalancerController) getServiceObject(namespace string, serviceName string) *api.Service {

	svcObj, svcExists, err := lbController.svcLister.Store.Get(&api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
	})
	if !svcExists {
		glog.Errorf("Service %v/%v not found in store", namespace, serviceName)
		return nil
	}
	if err != nil {
		glog.Errorf("Error getting service object %v/%v. %v", namespace, serviceName, err)
		return nil
	}
	return svcObj.(*api.Service)

}

func getServicePort(service *api.Service, targetPort int) (*api.ServicePort, error) {
	for _, p := range service.Spec.Ports {
		if int(p.Port) == targetPort {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("Could not find matching port %v from service %v.", targetPort, service.Name)
}

func nodeReady(node api.Node) bool {
	for ix := range node.Status.Conditions {
		condition := &node.Status.Conditions[ix]
		if condition.Type == api.NodeReady {
			return condition.Status == api.ConditionTrue
		}
	}
	return false
}

// getReadyNodeNames returns names of schedulable, ready nodes from the node lister.
func (lbController *LoadBalancerController) getReadyNodeIPs() ([]string, error) {
	nodeIPs := []string{}
	nodes, err := lbController.nodeLister.NodeCondition(nodeReady).List()
	if err != nil {
		return nodeIPs, err
	}
	for _, n := range nodes.Items {
		if n.Spec.Unschedulable {
			continue
		}
		ip, err := getNodeHostIP(n)
		if err != nil {
			glog.Errorf("Error getting node IP for %v. %v", n.Name, err)
			continue
		}
		nodeIPs = append(nodeIPs, *ip)
	}
	return nodeIPs, nil
}

// getNodeHostIP returns the provided node's IP, based on the priority:
// 1. NodeExternalIP
// 2. NodeLegacyHostIP
// 3. NodeInternalIP
func getNodeHostIP(node api.Node) (*string, error) {
	addresses := node.Status.Addresses
	addressMap := make(map[api.NodeAddressType][]api.NodeAddress)
	for i := range addresses {
		addressMap[addresses[i].Type] = append(addressMap[addresses[i].Type], addresses[i])
	}
	if addresses, ok := addressMap[api.NodeExternalIP]; ok {
		return &addresses[0].Address, nil
	}
	if addresses, ok := addressMap[api.NodeLegacyHostIP]; ok {
		return &addresses[0].Address, nil
	}
	if addresses, ok := addressMap[api.NodeInternalIP]; ok {
		return &addresses[0].Address, nil
	}
	return nil, fmt.Errorf("Host IP unknown; known addresses: %v", addresses)
}

func (lbController *LoadBalancerController) getLBConfigMapNodePortMap() map[string]int {
	configMapNodePortMap := make(map[string]int)
	configmaps := lbController.configMapLister.List()
	for _, obj := range configmaps {
		cm := obj.(*api.ConfigMap)
		cmData := cm.Data
		serviceName := cmData["target-service-name"]
		serviceObj := lbController.getServiceObject(cm.Namespace, serviceName)
		// Check if the service exists
		if serviceObj == nil {
			continue
		}

		targetServicePort, _ := strconv.Atoi(cmData["target-port"])
		servicePort, err := getServicePort(serviceObj, targetServicePort)
		if err != nil {
			glog.Errorf("Error while getting the service port %v", err)
			continue
		}

		configMapNodePortMap[cm.Namespace+"-"+cm.Name] = int(servicePort.NodePort)
	}
	return configMapNodePortMap
}
