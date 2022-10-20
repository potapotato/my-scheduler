# 自定义调度器

## 依赖 
- k8s集群
- go语言运行环境
- client-go

## 主要步骤
调度器要做的事情如下：
- 监控 pod 的 `ADDED` 事件
- 检测到 `ADDED` 事件后，编写自己的逻辑去选择合适的节点
- 将选择的节点和待调度的 pod 进行 bind 操作
- 发送 `Scheduled` 事件，以便监控

## 运行方式
```bash
go run randomScheduleV1.go
```

然后该调度器就开始监听了，接下来去编写一个 pod 

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: "myapp"
  namespace: default
  labels:
    app: "myapp"
spec:
  schedulerName: random-scheduler
  containers:
  - name: nginx
    image: "nginx"
    ports:
    - containerPort: 80
      name:  http
```

注意设置调度器名字即可

## 运行结果

![image-20221020111247548](assets\image-20221020111247548.png)

![image-20221020111309130](assets\image-20221020111309130.png)
