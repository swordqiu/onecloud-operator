package component

import (
	"fmt"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"yunion.io/x/onecloud-operator/pkg/apis/constants"
	"yunion.io/x/onecloud-operator/pkg/apis/onecloud/v1alpha1"
	"yunion.io/x/onecloud-operator/pkg/controller"
	"yunion.io/x/onecloud-operator/pkg/manager"
)

type hostDeployerManager struct {
	*ComponentManager
}

func newHostDeployerManger(man *ComponentManager) manager.Manager {
	return &hostDeployerManager{man}
}

func (m *hostDeployerManager) Sync(oc *v1alpha1.OnecloudCluster) error {
	return syncComponent(m, oc, oc.Spec.HostDeployer.Disable)
}

func (m *hostDeployerManager) getDaemonSet(
	oc *v1alpha1.OnecloudCluster,
	cfg *v1alpha1.OnecloudClusterConfig,
) (*apps.DaemonSet, error) {
	return m.newHostPrivilegedDaemonSet(v1alpha1.HostDeployerComponentType, oc, cfg)
}

func (m *hostDeployerManager) newHostPrivilegedDaemonSet(
	cType v1alpha1.ComponentType,
	oc *v1alpha1.OnecloudCluster,
	cfg *v1alpha1.OnecloudClusterConfig,
) (*apps.DaemonSet, error) {
	if err := ensureOptCloudExist(); err != nil {
		return nil, err
	}
	var (
		privileged  = true
		dsSpec      = oc.Spec.HostDeployer
		configMap   = controller.ComponentConfigMapName(oc, v1alpha1.HostComponentType)
		containersF = func(volMounts []corev1.VolumeMount) []corev1.Container {
			return []corev1.Container{
				{
					Name:  cType.String(),
					Image: dsSpec.Image,
					Command: []string{
						fmt.Sprintf("/opt/yunion/bin/%s", cType.String()),
						"--config",
						fmt.Sprintf("/etc/yunion/%s.conf", v1alpha1.HostComponentType.String()),
					},
					ImagePullPolicy: dsSpec.ImagePullPolicy,
					VolumeMounts:    volMounts,
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			}
		}
	)
	ds, err := m.newDaemonSet(cType, oc, cfg, NewHostVolume(cType, oc, configMap), dsSpec, nil, containersF)
	if err != nil {
		return nil, err
	}
	// set inter pod affinity
	if ds.Spec.Template.Spec.Affinity == nil {
		ds.Spec.Template.Spec.Affinity = &corev1.Affinity{}
	}
	if ds.Spec.Template.Spec.Affinity.PodAffinity == nil {
		ds.Spec.Template.Spec.Affinity.PodAffinity = &corev1.PodAffinity{}
	}
	ds.Spec.Template.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution = []corev1.PodAffinityTerm{
		corev1.PodAffinityTerm{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{constants.OnecloudHostDeployerLabelKey: ""},
			},
			TopologyKey: "kubernetes.io/hostname",
		},
	}
	return ds, nil
}
