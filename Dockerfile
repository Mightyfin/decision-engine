# syntax=docker/dockerfile:1
FROM golang:1.25.13-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY auth ./auth
COPY cmd ./cmd
COPY credit-risk ./credit-risk
COPY httpapi ./httpapi
COPY migrations ./migrations
COPY pricing ./pricing
COPY product ./product
COPY storage ./storage
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/decision-engine-api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/decision-engine-migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/decision-engine-api /decision-engine-api
COPY --from=build /out/decision-engine-migrate /decision-engine-migrate
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/decision-engine-api"]
