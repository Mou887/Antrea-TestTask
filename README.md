# Node-Scoped Packet Capture Controller

Hi,

This repository contains my implementation of the requested Kubernetes controller task.

---

## What This Does

- Runs as a **DaemonSet** (one Pod per node)
- Watches Pods scheduled on the same node
- Looks for the annotation:

  Capture is executed using:

  ``` tcpdump -C 1M -W <N> -w /captures/capture-<pod>.pcap ``` 

## What I Implemented

- SharedInformer for Pod watch
- Node filtering using `spec.nodeName`
- Annotation parsing and validation
- Start / Stop / Restart logic
- Cleanup of rotated pcap files
- Concurrency protection using `sync.Mutex`

## Build & Run

```make build ```

## Build image:

``` make docker-build ```


Deploy:
```
kubectl apply -f rbac.yaml
kubectl apply -f daemonset.yaml
kubectl apply -f test-pod.yaml
```

Thank you for reviewing.

**Moumita Dhar**
