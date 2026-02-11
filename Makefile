BINARY_NAME := packet-capture
IMAGE_NAME  := packet-capture-controller
IMAGE_TAG   := latest

build:
	go build -o $(BINARY_NAME) ./cmd

docker-build: build
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

docker-push:
	docker push $(IMAGE_NAME):$(IMAGE_TAG)

deploy:
	kubectl apply -f rbac.yaml
	kubectl apply -f daemonset.yaml

deploy-test:
	kubectl apply -f test-pod.yaml

delete-test:
	kubectl delete -f test-pod.yaml

clean:
	rm -f $(BINARY_NAME)
