package main


import (
	"fmt"
	"os"
        "os/exec"
        "sync"

        "strconv"
	"path/filepath"
"k8s.io/client-go/rest"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// getCaptureCount retrieves the value of the "tcpdump.antrea.io" annotation
// from the given Pod and attempts to parse it as a positive integer.
//
// It returns three values:
//   - int:  the parsed capture count (0 if not set or invalid)
//   - bool: whether the annotation is present on the Pod
//   - error: non-nil if the annotation exists but contains an invalid value
//
// If the annotation is not present, the function returns (0, false, nil).
// If the annotation is present but not a valid positive integer,
// it returns (0, true, error).

func getCaptureCount(pod *corev1.Pod) (int, bool, error) {
	val, ok := pod.Annotations["tcpdump.antrea.io"]
	if !ok {
		return 0, false, nil
	}

	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return 0, true, fmt.Errorf("invalid tcpdump.antrea.io value: %q", val)
	}

	return n, true, nil
}

type captureInfo struct {
    cmd *exec.Cmd
    n   int
}

var (
    captureProcs = make(map[string]*captureInfo)
    mu           sync.Mutex
)


func podKey(pod *corev1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

func startCapture(pod *corev1.Pod, n int) {
	key := podKey(pod)

	mu.Lock()
	defer mu.Unlock()

	if _, exists := captureProcs[key]; exists {
		fmt.Printf("[INFO] capture already running for %s\n", key)
		return
	}

	pcapPath := fmt.Sprintf("/captures/capture-%s.pcap", pod.Name)

	cmd := exec.Command(
		"tcpdump",
		"-C", "1M",
		"-W", strconv.Itoa(n),
		"-w", pcapPath,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("[ERROR] failed to start tcpdump for %s: %v\n", key, err)
		return
	}

	captureProcs[key] = &captureInfo{
    cmd: cmd,
    n:   n,
}

	fmt.Printf("[CAPTURE RUNNING] %s -> %s\n", key, pcapPath)
}

func stopCapture(pod *corev1.Pod) {
	key := podKey(pod)

	mu.Lock()
	defer mu.Unlock()

	info, exists := captureProcs[key]
	if !exists {
		fmt.Printf("[INFO] no running capture for %s\n", key)
		return
	}

	// Kill the tcpdump process
	if err := info.cmd.Process.Kill(); err != nil {
		fmt.Printf("[ERROR] failed to stop tcpdump for %s: %v\n", key, err)
	}

	// Delete rotated pcap files
	base := fmt.Sprintf("/captures/capture-%s.pcap", pod.Name)

	for i := 0; i < info.n; i++ {
		file := base
		if i > 0 {
			file = fmt.Sprintf("%s%d", base, i)
		}

		if err := os.Remove(file); err == nil {
			fmt.Printf("[PCAP DELETED] %s\n", file)
		}
	}

	
	delete(captureProcs, key)

	fmt.Printf("[CAPTURE STOPPED] %s\n", key)
}


func main() {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		panic("NODE_NAME must be set")
	}
	fmt.Printf("Controller running on node: %s\n", nodeName)

	var config *rest.Config
var err error

// Try in-cluster config first (DaemonSet / Pod case)
config, err = rest.InClusterConfig()
if err != nil {
	// Fallback to local kubeconfig (go run case)
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err.Error())
	}
}


	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	factory := informers.NewSharedInformerFactory(clientset, 0)
	podInformer := factory.Core().V1().Pods().Informer()

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
	pod := obj.(*corev1.Pod)
	if pod.Spec.NodeName != nodeName {
		return
	}

	n, ok, err := getCaptureCount(pod)
	if err != nil {
		fmt.Printf("[ERROR] %s/%s: %v\n", pod.Namespace, pod.Name, err)
		return
	}

	if ok {
		fmt.Printf("[CAPTURE REQUESTED] %s/%s N=%d\n", pod.Namespace, pod.Name, n)
	}
},

		UpdateFunc: func(oldObj, newObj interface{}) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	// Only care about Pods on the same node
	if newPod.Spec.NodeName != nodeName {
		return
	}

	oldN, oldOk, _ := getCaptureCount(oldPod)
	newN, newOk, err := getCaptureCount(newPod)

	if err != nil {
		fmt.Printf("[ERROR] %s/%s: %v\n", newPod.Namespace, newPod.Name, err)
		return
	}

	// Annotation added → start capture
	if !oldOk && newOk {
		fmt.Printf("[CAPTURE START] %s/%s N=%d\n",
			newPod.Namespace, newPod.Name, newN)
		startCapture(newPod, newN)
	}

	// Annotation removed → stop capture
	if oldOk && !newOk {
		fmt.Printf("[CAPTURE STOP] %s/%s\n",
			newPod.Namespace, newPod.Name)
		stopCapture(newPod)
	}

	// Annotation value changed → restart capture (optional but correct)
	if oldOk && newOk && oldN != newN {
		fmt.Printf("[CAPTURE UPDATE] %s/%s N=%d\n",
			newPod.Namespace, newPod.Name, newN)
		stopCapture(newPod)
		startCapture(newPod, newN)
	}
},


		DeleteFunc: func(obj interface{}) {
	pod := obj.(*corev1.Pod)
	if pod.Spec.NodeName != nodeName {
		return
	}

	fmt.Printf("[DELETE] %s/%s\n", pod.Namespace, pod.Name)
	stopCapture(pod)
},

	})

	stopCh := make(chan struct{})
	factory.Start(stopCh)

	if !cache.WaitForCacheSync(stopCh, podInformer.HasSynced) {
		panic("Failed to sync caches")
	}

	fmt.Println("Pod informer running...")
	for {
		time.Sleep(10 * time.Second)
	}
}



