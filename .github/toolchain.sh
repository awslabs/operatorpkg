#!/usr/bin/env bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:="1.31.x"}"
KUBEBUILDER_ASSETS="/usr/local/kubebuilder/bin"

main() {
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
    
    kubebuilder
}

kubebuilder() {
    if ! mkdir -p ${KUBEBUILDER_ASSETS}; then
      sudo mkdir -p ${KUBEBUILDER_ASSETS}
      sudo chown $(whoami) ${KUBEBUILDER_ASSETS}
    fi
    arch=$(go env GOARCH)
    ln -sf $(setup-envtest use -p path "${K8S_VERSION}" --arch="${arch}" --bin-dir="${KUBEBUILDER_ASSETS}")/* ${KUBEBUILDER_ASSETS}
    find $KUBEBUILDER_ASSETS

    # Install latest binaries for 1.25.x (contains CEL fix)
    if [[ "${K8S_VERSION}" = "1.25.x" ]] && [[ "$OSTYPE" == "linux"* ]]; then
        for binary in 'kube-apiserver' 'kubectl'; do
            rm $KUBEBUILDER_ASSETS/$binary
            wget -P $KUBEBUILDER_ASSETS dl.k8s.io/v1.25.16/bin/linux/${arch}/${binary}
            chmod +x $KUBEBUILDER_ASSETS/$binary
        done
    fi
}

main "$@"
