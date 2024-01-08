FROM registry.access.redhat.com/ubi9/go-toolset:1.20

WORKDIR /qe-tools

RUN git clone --branch main https://github.com/redhat-appstudio/qe-tools.git . && \
    make build

