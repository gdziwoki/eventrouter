# Copyright 2017 Heptio Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM --platform=$BUILDPLATFORM golang:1.20-alpine3.18@sha256:7839c9f01b5502d7cb5198b2c032857023424470b3e31ae46a8261ffca72912a AS builder

RUN apk add --update --no-cache ca-certificates make git curl

ARG TARGETOS
ARG TARGETARCH
ARG TARGETPLATFORM

WORKDIR /app

ARG GOPROXY

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY *.go /app/
COPY sinks/ /app/sinks/
COPY Makefile /app/Makefile

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH make build

FROM gcr.io/distroless/static:latest@sha256:7198a357ff3a8ef750b041324873960cf2153c11cc50abb9d8d5f8bb089f6b4e

COPY --from=builder /app/eventrouter /app/eventrouter

CMD ["/app/eventrouter", "-v=3", "-logtostderr"]
