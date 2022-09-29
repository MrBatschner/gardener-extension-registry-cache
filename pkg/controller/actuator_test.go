// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	"github.com/gardener/gardener-extension-registry-cache/pkg/apis/registry/v1alpha1"
	extensionsconfig "github.com/gardener/gardener/extensions/pkg/apis/config"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	registryDocker = "docker.io"
	registryGcr    = "gcr.io"
)

var _ = Describe("Actuator", func() {

	Describe("Actuator tests for extension config", func() {
		var (
			ctx = context.TODO()

			scheme          *runtime.Scheme
			logger          logr.Logger
			fakeClient      client.Client
			fakeShootClient client.Client
			a               actuator
			ex              *extensionsv1alpha1.Extension

			namespace = "foo"

			registryConfig v1alpha1.RegistryConfig

			fakeShootClientFn = func(_ context.Context, _ client.Client, _ string, _ client.Options, _ extensionsconfig.RESTOptions) (*rest.Config, client.Client, error) {
				return nil, fakeShootClient, nil
			}
		)

		BeforeEach(func() {
			logger = log.Log.WithName("test")

			cluster := &extensionsv1alpha1.Cluster{
				ObjectMeta: v1.ObjectMeta{
					Name: "foo",
				},
			}

			scheme = runtime.NewScheme()
			schemeBuilder := runtime.NewSchemeBuilder(
				resourcesv1alpha1.AddToScheme,
				extensionsv1alpha1.AddToScheme,
				v1alpha1.AddToScheme,
				corev1.AddToScheme,
			)
			utilruntime.Must(schemeBuilder.AddToScheme(scheme))

			fakeClient = fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
			fakeShootClient = fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()

			a = actuator{
				client:  fakeClient,
				decoder: serializer.NewCodecFactory(scheme, serializer.EnableStrict).UniversalDecoder(),
			}

			defaultCacheSize := resource.MustParse("10Gi")

			registryConfig = v1alpha1.RegistryConfig{
				TypeMeta: v1.TypeMeta{
					APIVersion: v1alpha1.SchemeGroupVersion.String(),
					Kind:       "RegistryConfig",
				},
				Caches: []v1alpha1.RegistryCache{
					{
						Upstream:                 registryGcr,
						Size:                     &defaultCacheSize,
						GarbageCollectionEnabled: pointer.Bool(true),
					},
				},
			}

			ex = &extensionsv1alpha1.Extension{
				ObjectMeta: v1.ObjectMeta{
					Namespace: namespace,
				},
				Spec: extensionsv1alpha1.ExtensionSpec{
					DefaultSpec: extensionsv1alpha1.DefaultSpec{
						Type: "registry-cache",
						ProviderConfig: &runtime.RawExtension{
							Raw: encodeRegistryConfig(&registryConfig),
						},
					},
				},
			}
		})

		It("should do nothing because no extension config found", func() {
			ex.Spec.ProviderConfig = nil

			err := a.Reconcile(ctx, logger, ex)
			Expect(err).NotTo(HaveOccurred())

			mrl := &resourcesv1alpha1.ManagedResourceList{}
			err = a.client.List(ctx, mrl, &client.ListOptions{Namespace: ex.GetNamespace()})
			Expect(err).ToNot(HaveOccurred())
			Expect(mrl).NotTo(BeNil())
			Expect(mrl.Items).To(HaveLen(0))
		})

		It("should create managed resources for registry and cri-ensurer", func() {
			oldShootClient := NewClientForShoot
			NewClientForShoot = fakeShootClientFn
			defer func() { NewClientForShoot = oldShootClient }()

			err := a.Reconcile(ctx, logger, ex)
			Expect(err).NotTo(HaveOccurred())

			mrl := &resourcesv1alpha1.ManagedResourceList{}
			err = a.client.List(ctx, mrl, &client.ListOptions{Namespace: ex.GetNamespace()})
			Expect(err).ToNot(HaveOccurred())
			Expect(mrl).NotTo(BeNil())
			Expect(mrl.Items).To(HaveLen(2))

			secl := &corev1.SecretList{}
			err = a.client.List(ctx, secl, &client.ListOptions{Namespace: ex.GetNamespace()})
			Expect(err).ToNot(HaveOccurred())
			Expect(secl).NotTo(BeNil())
			Expect(secl.Items).To(HaveLen(5))
		})
	})
})

func encodeRegistryConfig(c runtime.Object) []byte {
	data, _ := json.Marshal(c)
	return data
}
