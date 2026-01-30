// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"path"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/pkg/errors"

	"yunion.io/x/onecloud-operator/pkg/apis/constants"
	"yunion.io/x/onecloud-operator/pkg/apis/onecloud/v1alpha1"
	"yunion.io/x/onecloud-operator/pkg/controller"
	"yunion.io/x/onecloud-operator/pkg/manager"
	"yunion.io/x/onecloud-operator/pkg/util/onecloud"
	"yunion.io/x/onecloud-operator/pkg/util/option"
)

type cloudDesktopManager struct {
	*ComponentManager
}

func newcloudDesktopManager(man *ComponentManager) manager.ServiceManager {
	return &cloudDesktopManager{man}
}

func (m *cloudDesktopManager) getProductVersions() []v1alpha1.ProductVersion {
	return []v1alpha1.ProductVersion{
		v1alpha1.ProductVersionFullStack,
		v1alpha1.ProductVersionCMP,
		v1alpha1.ProductVersionEdge,
	}
}

func (m *cloudDesktopManager) GetComponentType() v1alpha1.ComponentType {
	return v1alpha1.CloudDesktopComponentType
}

func (m *cloudDesktopManager) IsDisabled(oc *v1alpha1.OnecloudCluster) bool {
	return oc.Spec.CloudDesktop.Disable
}

func (m *cloudDesktopManager) GetServiceName() string {
	return constants.ServiceNameCloudDesktop
}

func (m *cloudDesktopManager) Sync(oc *v1alpha1.OnecloudCluster) error {
	return syncComponent(m, oc, "")
}

func (m *cloudDesktopManager) getDBConfig(cfg *v1alpha1.OnecloudClusterConfig) *v1alpha1.DBConfig {
	return &cfg.CloudDesktop.DB
}

func (m *cloudDesktopManager) getDBEngine(oc *v1alpha1.OnecloudCluster) v1alpha1.TDBEngineType {
	return oc.Spec.GetDbEngine(oc.Spec.CloudDesktop.DbEngine)
}

func (m *cloudDesktopManager) getCloudUser(cfg *v1alpha1.OnecloudClusterConfig) *v1alpha1.CloudUser {
	return &cfg.CloudDesktop.CloudUser
}

func (m *cloudDesktopManager) getPhaseControl(man controller.ComponentManager, zone string) controller.PhaseControl {
	return controller.NewRegisterEndpointComponent(man, v1alpha1.CloudDesktopComponentType,
		constants.ServiceNameCloudDesktop, constants.ServiceTypeCloudDesktop,
		man.GetCluster().Spec.CloudDesktop.Service.NodePort, "")
}

type cloudDesktopOptions struct {
	v1alpha1.CloudDesktopCommonConfig

	option.CommonDBOptions
}

func (m *cloudDesktopManager) getConfigMap(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string) (*corev1.ConfigMap, bool, error) {
	opt := &cloudDesktopOptions{}
	if err := option.SetOptionsDefault(opt, constants.ServiceTypeCloudDesktop); err != nil {
		return nil, false, err
	}
	config := cfg.CloudDesktop

	switch oc.Spec.GetDbEngine(oc.Spec.CloudDesktop.DbEngine) {
	case v1alpha1.DBEngineDameng:
		option.SetDamengOptions(&opt.DBOptions, oc.Spec.Dameng, config.DB)
	case v1alpha1.DBEngineMySQL:
		fallthrough
	default:
		option.SetMysqlOptions(&opt.DBOptions, oc.Spec.Mysql, config.DB)
	}

	option.SetOptionsServiceTLS(&opt.BaseOptions, false)
	option.SetServiceCommonOptions(&opt.CommonOptions, oc, config.ServiceCommonOptions, cfg.CommonConfig)
	// opt.AutoSyncTable = true
	opt.SslCertfile = path.Join(constants.CertDir, constants.ServiceCertName)
	opt.SslKeyfile = path.Join(constants.CertDir, constants.ServiceKeyName)
	opt.Port = config.Port

	opt.CloudDesktopCommonConfig = config.CloudDesktopCommonConfig
	if opt.HostTcpPortStart == 0 {
		opt.HostTcpPortStart = 20001
	}
	if opt.HostTcpPortEnd == 0 {
		opt.HostTcpPortEnd = 24999
	}
	if opt.HostUdpPortStart == 0 {
		opt.HostUdpPortStart = 20001
	}
	if opt.HostUdpPortEnd == 0 {
		opt.HostUdpPortEnd = 24999
	}

	return m.newServiceConfigMap(v1alpha1.CloudDesktopComponentType, "", oc, opt), false, nil
}

func (m *cloudDesktopManager) getService(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string) []*corev1.Service {
	return m.newSinglePortService(v1alpha1.CloudDesktopComponentType, oc, oc.Spec.CloudDesktop.Service.InternalOnly, int32(oc.Spec.CloudDesktop.Service.NodePort), int32(cfg.CloudDesktop.Port), oc.Spec.CloudDesktop.SlaveReplicas > 0)
}

func (m *cloudDesktopManager) getDeployment(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string) (*apps.Deployment, error) {
	deploy, err := m.newCloudServiceSinglePortDeploymentWithReadinessProbePath(
		v1alpha1.CloudDesktopComponentType, "", oc, &oc.Spec.CloudDesktop.DeploymentSpec,
		int32(cfg.CloudDesktop.Port), true, false,
		"/ping",
	)
	if err != nil {
		return nil, errors.Wrap(err, "newCloudServiceSinglePortDeploymentWithReadinessProbePath")
	}

	podTemplate := &deploy.Spec.Template.Spec
	podVols := podTemplate.Volumes
	volMounts := podTemplate.Containers[0].VolumeMounts

	var (
		hostPathDirOrCreate = corev1.HostPathDirectoryOrCreate
	)
	podVols = append(podVols, corev1.Volume{
		Name: "opt-cloud",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/opt/cloud",
				Type: &hostPathDirOrCreate,
			},
		},
	})
	propagation := corev1.MountPropagationHostToContainer
	volMounts = append(volMounts, corev1.VolumeMount{
		Name:             "opt-cloud",
		MountPath:        "/opt/cloud",
		MountPropagation: &propagation,
	})

	podTemplate.Containers[0].VolumeMounts = volMounts
	podTemplate.Volumes = podVols

	return deploy, nil
}

func (m *cloudDesktopManager) getDeploymentStatus(oc *v1alpha1.OnecloudCluster, zone string) *v1alpha1.DeploymentStatus {
	return &oc.Status.CloudDesktop
}

func (m *cloudDesktopManager) supportsReadOnlyService() bool {
	return true
}

func (m *cloudDesktopManager) getReadonlyDeployment(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string, deployment *apps.Deployment) *apps.Deployment {
	return m.genReadonlyDeployment(v1alpha1.CloudDesktopComponentType, oc, deployment, &oc.Spec.CloudDesktop.DeploymentSpec)
}

func (m *cloudDesktopManager) getMcclientSyncFunc(oc *v1alpha1.OnecloudCluster) func(*mcclient.ClientSession) error {
	return func(s *mcclient.ClientSession) error {
		if m.IsDisabled(oc) {
			return onecloud.EnsureDisableService(s, m.GetServiceName())
		} else {
			return onecloud.EnsureEnableService(s, m.GetServiceName(), m.supportsReadOnlyService() && oc.Spec.CloudDesktop.SlaveReplicas > 0)
		}
	}
}
