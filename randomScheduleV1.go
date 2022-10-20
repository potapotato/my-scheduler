package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	schedulerName = "random-scheduler"
)

func main() {
	// 创建配置
	config, err := clientcmd.BuildConfigFromFlags("", "config")
	if err != nil {
		panic(err.Error())
	}

	// 创建 clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// watch
	watch, err := clientset.CoreV1().Pods("").Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.schedulerName=%s,spec.nodeName=", schedulerName),
	})
	if err != nil {
		panic(err.Error())
	}

	// 等待 ADDED 事件
	for event := range watch.ResultChan() {
		if event.Type != "ADDED" {
			continue
		}
		p := event.Object.(*corev1.Pod)
		fmt.Println("found a pod to schedule:", p.Namespace, "/", p.Name)

		// 调度
		scheduledNode, err := ChooseFitNode(clientset)
		if err != nil {
			panic(err.Error())
		}

		// bind 操作
		clientset.CoreV1().Pods(p.Namespace).Bind(context.TODO(), &corev1.Binding{
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
		clientset.CoreV1().Events(p.Namespace).Create(context.TODO(), &corev1.Event{
			Count:          1,
			Message:        message,
			Reason:         "Scheduled",
			LastTimestamp:  metav1.NewTime(timestamp),
			FirstTimestamp: metav1.NewTime(timestamp),
			Type:           "Normal",
			Source: v1.EventSource{
				Component: schedulerName,
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
}

func ChooseFitNode(clientset *kubernetes.Clientset) (*corev1.Node, error) {
	nodes, _ := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("no nodes can be scheduled")
	}

	return &nodes.Items[rand.Intn(len(nodes.Items))], nil
}
