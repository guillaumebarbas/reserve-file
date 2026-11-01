FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api
# distroless has no shell, so the data directory is prepared here and copied.
RUN mkdir -p /out/data

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/api /app/api
# A named volume mounted on /data starts with this ownership, so the non-root
# process can write its database there.
COPY --from=build --chown=65532:65532 /out/data /data
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]
