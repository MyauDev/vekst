module github.com/MyauDev/vekst

go 1.26

require (
	connectrpc.com/connect v1.19.1
	google.golang.org/grpc v1.81.0
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/go-chi/chi/v5 v5.3.2 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
)

tool (
	connectrpc.com/connect/cmd/protoc-gen-connect-go
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
