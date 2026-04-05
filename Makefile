BUILD_DIR = build
CFLAGS = -ldflags="-s -w"
ROUTER_IP = 172.16.0.4
GO_VERSION = 1.22.10

run: 
	go$(GO_VERSION) run app.go

run-local: build-local
	$(BUILD_DIR)/app

build-local: clean
	mkdir -p $(BUILD_DIR)
	go$(GO_VERSION) build -o $(BUILD_DIR)/app app.go

build-mips: clean
	mkdir -p $(BUILD_DIR)
	GOARCH=mips GOOS=linux GOMIPS=softfloat CGO_ENABLED=0 go$(GO_VERSION) build $(CFLAGS) -o $(BUILD_DIR)/app-mips app.go
	upx --lzma $(BUILD_DIR)/app-mips

clean:
	rm -rf $(BUILD_DIR)

deploy-router: build-mips
	scp -O $(BUILD_DIR)/app-mips root@$(ROUTER_IP):/tmp
	
