# Load balancer using configmap

The project implements a load balancer controller that will provide access and load balancing to HTTP and TCP applications. It also provide SSL support for http apps.

Our goal is to have this controller listen to ingress events, rather than config map for generating config rules. This is still being planned in kubernetes since the current version
of ingress does not support layer 4 routing.

It is designed to easily integrate and create different load balancing backends. Our initial backend is nginx.

## Examples

### HTTP Load Balancing:
1. Create coffee web app, which consists of a service and replication controller resource:

  ```
  $ kubectl create -f examples/coffee-app.yaml
  ```

1. Create controller Resource:
  ```
  $ kubectl create -f ingress-loadbalancer-rc.yaml
  ```

1. The Controller container exposes ports 80 and 3306 on the host it runs. 
Make sure to add a firewall to allow incoming traffic on this ports.

1. Create configmap for the coffee service:
  ```
  $ kubectl create -f coffee-configmap.yaml
  ```

1. Find out the node/external IP address of the node of the controller:
  ```
  $ kubectl get pods -o wide
  NAME                          READY     STATUS    RESTARTS   AGE       NODE
  coffee-rc-mtjuw               1/1       Running   0          3m        172.17.4.202
  coffee-rc-mu9ns               1/1       Running   0          3m        172.17.4.202
  ingress-loadbalancer-rc-9p6ay 1/1       Running   0          1m        172.17.4.201
  mysql-pod                     1/1       Running   0          1h        172.17.4.202
  ```

  ```
  $ kubectl get node 172.17.4.201 -o json | grep -A 2 ExternalIP
      "type": "ExternalIP",
    "address": "XXX.YYY.ZZZ.III"
    }
  ```

1. We'll use ```curl```'s --resolve option to set the Host header of a request with ```foo.bar.com```.
   To get coffee:
  ```
  $ curl --resolve foo.bar.com:80:XXX.YYY.ZZZ.III http://foo.bar.com/coffee
  <!DOCTYPE html>
  <html>
  <head>
  <title>Hello from NGINX!</title>
  <style>
      body {
          width: 35em;
          margin: 0 auto;
          font-family: Tahoma, Verdana, Arial, sans-serif;
      }
  </style>
  </head>
  <body>
  <h1>Hello!</h1>
  <h2>URI = /coffee</h2>
  <h2>My hostname is coffee-rc-mu9ns</h2>
  <h2>My address is 10.244.0.3:80</h2>
  </body>
  </html>
  ```

### TCP Load Balancing:
1. Create mysql app:

  ```
  $ kubectl create -f examples/mysql-app.yaml
  ```

1. Create controller Resource (if you havent had already created it):
  ```
  $ kubectl create -f ingress-loadbalancer-rc.yaml
  ```

1. The Controller container exposes ports 80 and 3306 on the host it runs. 
Make sure to add a firewall to allow incoming traffic on this ports.

1. Create configmap for the mysql service:
  ```
  $ kubectl create -f mysql-configmap.yaml
  ```

1. Find out the node/external IP address of the node of the controller:
  ```
  $ kubectl get pods -o wide
  NAME                          READY     STATUS    RESTARTS   AGE       NODE
  coffee-rc-mtjuw               1/1       Running   0          3m        172.17.4.202
  coffee-rc-mu9ns               1/1       Running   0          3m        172.17.4.202
  ingress-loadbalancer-rc-9p6ay 1/1       Running   0          1m        172.17.4.201
  mysql-pod                     1/1       Running   0          1h        172.17.4.202
  ```

  ```
  $ kubectl get node 172.17.4.201 -o json | grep -A 2 ExternalIP
      "type": "ExternalIP",
    "address": "XXX.YYY.ZZZ.III"
    }
  ```

1. We'll use the mysql client to connect to the mysql app:
  ```
  $ mysql -h XXX.YYY.ZZZ.III -u mysql -pmysql
  Warning: Using a password on the command line interface can be insecure.
  Welcome to the MySQL monitor.  Commands end with ; or \g.
  Your MySQL connection id is 7
  Server version: 5.7.12 MySQL Community Server (GPL)

  Copyright (c) 2000, 2014, Oracle and/or its affiliates. All rights reserved.

  Oracle is a registered trademark of Oracle Corporation and/or its
  affiliates. Other names may be trademarks of their respective
  owners.

  Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

  mysql>
  ```


**Note**: Implementations are experimental and not suitable for using in production. This project is still in its early stage and many things are still in work in progress.
