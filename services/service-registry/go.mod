// services/service-registry/go.mod
module github.com/Medex81/ProjectorBackend/services/service-registry

go 1.25.7

require (
    github.com/Medex81/ProjectorBackend/pkg/common v0.0.0
    github.com/gorilla/mux v1.8.1
    google.golang.org/grpc v1.69.4
)

replace github.com/Medex81/ProjectorBackend/pkg/common => ../../pkg/common