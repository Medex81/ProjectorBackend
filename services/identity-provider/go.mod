// services/identity-provider/go.mod
module github.com/Medex81/ProjectorBackend/services/identity-provider

go 1.25.7

require (
    github.com/Medex81/ProjectorBackend/pkg/common v0.0.0
    github.com/gorilla/mux v1.8.1
    github.com/jackc/pgx/v5 v5.7.2
    golang.org/x/crypto v0.32.0
    google.golang.org/grpc v1.69.4
)

replace github.com/Medex81/ProjectorBackend/pkg/common => ../../pkg/common