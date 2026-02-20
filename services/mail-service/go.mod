// services/mail-service/go.mod
module github.com/Medex81/ProjectorBackend/services/mail-service

go 1.25.7

require (
    github.com/Medex81/ProjectorBackend/pkg/common v0.0.0
    github.com/gorilla/mux v1.8.1
    gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
)

replace github.com/Medex81/ProjectorBackend/pkg/common => ../../pkg/common