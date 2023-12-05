package test

import (
	"github.com/samber/lo"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type Environment struct {
	envtest.Environment
	Client              client.Client
	KubernetesInterface kubernetes.Interface
}

type EnvironmentOption func(env *envtest.Environment)

func EnvironmentOptionWithCRDs(crds []*apiextensionsv1.CustomResourceDefinition) func(env *envtest.Environment) {
	return func(env *envtest.Environment) {
		env.CRDs = crds
	}
}

func NewEnvironment(options ...EnvironmentOption) *Environment {
	environment := envtest.Environment{Scheme: scheme.Scheme}
	for _, option := range options {
		option(&environment)
	}
	config := lo.Must(environment.Start())
	return &Environment{
		Client:              lo.Must(client.New(config, client.Options{Scheme: scheme.Scheme})),
		KubernetesInterface: kubernetes.NewForConfigOrDie(config),
	}
}
