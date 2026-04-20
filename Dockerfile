FROM --platform=${TARGETPLATFORM:-linux/amd64} golang:1.26@sha256:5e69504bb541a36b859ee1c3daeb670b544b74b3dfd054aea934814e9107f6eb AS build-env

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
