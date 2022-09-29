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
	"fmt"
	"strconv"
	"strings"

	"github.com/gardener/gardener/pkg/utils/imagevector"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

var _ = Describe("Registry Deployer", func() {

	var (
		c      registryCache
		labels map[string]string

		sts     *appsv1.StatefulSet
		service *corev1.Service
	)

	const (
		registryUpstreamGCR    = "gcr.io"
		registryUpStreamDocker = "docker.io"

		referenceid = "registry-cache"

		registryImage = "registryImageFoo:bar"
	)

	BeforeEach(func() {
		format.MaxLength = 15000
		cacheSize := resource.MustParse("10Gi")

		labels = map[string]string{
			"foo":                             "bar",
			registryCacheServiceUpstreamLabel: registryUpstreamGCR,
		}

		c = registryCache{
			Name:                     "foo",
			Namespace:                "bar",
			Labels:                   labels,
			Upstream:                 registryUpstreamGCR,
			VolumeSize:               cacheSize,
			GarbageCollectionEnabled: true,
			RegistryImage: &imagevector.Image{
				Repository: registryImage,
			},
		}

		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName(registryUpstreamGCR),
				Namespace: registryCacheNamespaceName,
				Labels:    labels,
			},
			Spec: corev1.ServiceSpec{
				Selector: labels,
				Ports: []corev1.ServicePort{
					{
						Name:       referenceid,
						Port:       5000,
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromString(referenceid),
					},
				},
				Type: corev1.ServiceTypeClusterIP,
			},
		}

		sts = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName(registryUpstreamGCR),
				Namespace: registryCacheNamespaceName,
				Labels:    labels,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas:    pointer.Int32(1),
				ServiceName: resourceName(registryUpstreamGCR),
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            referenceid,
								Image:           registryImage,
								ImagePullPolicy: corev1.PullIfNotPresent,
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 5000,
										Name:          referenceid,
									},
								},
								Env: []corev1.EnvVar{
									{
										Name:  "REGISTRY_PROXY_REMOTEURL",
										Value: fmt.Sprintf("https://%s", registryUpstreamGCR),
									},
									{
										Name:  "REGISTRY_STORAGE_DELETE_ENABLED",
										Value: strconv.FormatBool(true),
									},
								},
								VolumeMounts: []v1.VolumeMount{
									{
										Name:      "cache-volume",
										ReadOnly:  false,
										MountPath: "/var/lib/registry",
									},
								},
							},
						},
					},
				},
				VolumeClaimTemplates: []v1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "cache-volume",
							Labels: labels,
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceStorage: cacheSize,
								},
							},
						},
					},
				},
			},
		}
	})

	It("Should create a Service and a Statefulset for a registry", func() {
		objects, err := c.Ensure()
		Expect(err).ToNot(HaveOccurred())
		Expect(objects).To(HaveLen(2))

		Expect(objects).To(ContainElements(service, sts))
	})

})

func resourceName(s string) string {
	return strings.Replace(fmt.Sprintf("registry-%s", s), ".", "-", -1)
}
