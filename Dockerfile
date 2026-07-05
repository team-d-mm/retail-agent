# syntax=docker/dockerfile:1

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /retail-agent ./cmd/retail-agent

FROM gcr.io/distroless/base-debian12
COPY --from=build /retail-agent /retail-agent
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/retail-agent"]
CMD ["serve-web"]
