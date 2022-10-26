package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Schedule struct {
	scheduleName    string
	client          *kubernetes.Clientset
	informerFactory informers.SharedInformerFactory
	queue           chan *v1.Pod
}

func NewSchedule() (*Schedule, error) {
	sched := &Schedule{}
	sched.scheduleName = "random-scheduler"
	sched.queue = make(chan *v1.Pod, 5)

	// 创建配置
	config, err := clientcmd.BuildConfigFromFlags("", "config")
	if err != nil {
		panic(err.Error())
	}

	// 创建 clientset
	sched.client, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// 获取 Informer 工厂
	sched.informerFactory = informers.NewSharedInformerFactory(sched.client, time.Second*30)

	// 获取 pod 的informer
	podInformer := sched.informerFactory.Core().V1().Pods()
	nodeInformer := sched.informerFactory.Core().V1().Nodes()

	println("start...")
	// 绑定回调事件
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// ADDED
		AddFunc: func(obj interface{}) {
			p := obj.(*v1.Pod)
			if p.Spec.NodeName == "" && p.Spec.SchedulerName == sched.scheduleName {
				fmt.Println("found a pod to schedule:", p.Namespace, "/", p.Name)
				sched.queue <- p
			}
		},
	})

	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			fmt.Printf("New Node Added to Store: %s\n", node.GetName())
		},
	})

	// 启动Informer工厂中的Informer
	sched.informerFactory.Start(wait.NeverStop)
	// 等待缓存数据和APIServer数据同步
	sched.informerFactory.WaitForCacheSync(wait.NeverStop)
	return sched, nil
}

func ChooseFitNode(nodes []*corev1.Node) (*corev1.Node, error) {
	rand.Seed(time.Now().UnixNano())
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes can be scheduled")
	}

	return nodes[rand.Intn(len(nodes))], nil
}

func (sched Schedule) schedule_one() {
	p := <-sched.queue

	// 调度
	nodeLister := sched.informerFactory.Core().V1().Nodes().Lister()
	nodes, err := nodeLister.List(labels.Everything())
	if err != nil {
		panic(err.Error())
	}

	scheduledNode, err := ChooseFitNode(nodes)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("schedule the pod to %s\n", scheduledNode.Name)

	// bind 操作
	sched.client.CoreV1().Pods(p.Namespace).Bind(context.TODO(), &corev1.Binding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.Name,
			Namespace: p.Namespace,
		},
		Target: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Node",
			Name:       scheduledNode.Name,
		},
	}, metav1.CreateOptions{})

	// 发送一个 `Scheduled` 事件，以便监控
	timestamp := time.Now().UTC()
	message := fmt.Sprintf("schedule pod %s/%s to node %s\n", p.Namespace, p.Name, scheduledNode.Name)
	sched.client.CoreV1().Events(p.Namespace).Create(context.TODO(), &corev1.Event{
		Count:          1,
		Message:        message,
		Reason:         "Scheduled",
		LastTimestamp:  metav1.NewTime(timestamp),
		FirstTimestamp: metav1.NewTime(timestamp),
		Type:           "Normal",
		Source: v1.EventSource{
			Component: sched.scheduleName,
		},
		InvolvedObject: v1.ObjectReference{
			Kind:      "Pod",
			Name:      p.Name,
			Namespace: p.Namespace,
			UID:       p.UID,
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: p.Name + "-",
		},
	}, metav1.CreateOptions{})
}

func main() {
	scheduler, err := NewSchedule()
	if err != nil {
		panic(err.Error())
	}

	stopCh := make(chan struct{})
	wait.Until(scheduler.schedule_one, 0, stopCh)
}
