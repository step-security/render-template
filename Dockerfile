FROM --platform=${TARGETPLATFORM:-linux/amd64} golang:1.26@sha256:d7722066f0b60ceccb6c0643cbed1f5f9e15506ac237146c95333504d7805d89 AS build-env

WORKDIR /go/src/app
ADD . /go/src/app

RUN go test -mod=vendor -cover ./...
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s" -mod=vendor -o /go/bin/app


FROM --platform=${TARGETPLATFORM:-linux/amd64} gcr.io/distroless/static@sha256:47b2d72ff90843eb8a768b5c2f89b40741843b639d065b9b937b07cd59b479c6

LABEL name="render-template"
LABEL repository="http://github.com/step-security/render-template"
LABEL homepage="http://github.com/step-security/render-template"

LABEL maintainer="step-security"
LABEL com.github.actions.name="Render template"
LABEL com.github.actions.description="Renders file based on template and passed variables"
LABEL com.github.actions.icon="file-text"
LABEL com.github.actions.color="purple"

COPY --from=build-env /go/bin/app /app

CMD ["/app"]
