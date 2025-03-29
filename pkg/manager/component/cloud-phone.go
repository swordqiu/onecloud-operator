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

	"yunion.io/x/pkg/errors"

	"yunion.io/x/onecloud-operator/pkg/apis/constants"
	"yunion.io/x/onecloud-operator/pkg/apis/onecloud/v1alpha1"
	"yunion.io/x/onecloud-operator/pkg/controller"
	"yunion.io/x/onecloud-operator/pkg/manager"
	"yunion.io/x/onecloud-operator/pkg/util/option"
)

type cloudPhoneManager struct {
	*ComponentManager
}

func newcloudPhoneManager(man *ComponentManager) manager.ServiceManager {
	return &cloudPhoneManager{man}
}

func (m *cloudPhoneManager) getProductVersions() []v1alpha1.ProductVersion {
	return []v1alpha1.ProductVersion{
		v1alpha1.ProductVersionFullStack,
		v1alpha1.ProductVersionEdge,
	}
}

func (m *cloudPhoneManager) GetComponentType() v1alpha1.ComponentType {
	return v1alpha1.CloudPhoneComponentType
}

func (m *cloudPhoneManager) IsDisabled(oc *v1alpha1.OnecloudCluster) bool {
	return oc.Spec.CloudPhone.Disable
}

func (m *cloudPhoneManager) GetServiceName() string {
	return constants.ServiceNameCloudPhone
}

func (m *cloudPhoneManager) Sync(oc *v1alpha1.OnecloudCluster) error {
	if len(oc.Spec.CloudPhone.ImageName) == 0 {
		oc.Spec.CloudPhone.ImageName = "cloud-phone"
	}
	return syncComponent(m, oc, "")
}

func (m *cloudPhoneManager) getDBConfig(cfg *v1alpha1.OnecloudClusterConfig) *v1alpha1.DBConfig {
	return &cfg.CloudPhone.DB
}

func (m *cloudPhoneManager) getDBEngine(oc *v1alpha1.OnecloudCluster) v1alpha1.TDBEngineType {
	return oc.Spec.GetDbEngine(oc.Spec.CloudPhone.DbEngine)
}

func (m *cloudPhoneManager) getCloudUser(cfg *v1alpha1.OnecloudClusterConfig) *v1alpha1.CloudUser {
	return &cfg.CloudPhone.CloudUser
}

func (m *cloudPhoneManager) getPhaseControl(man controller.ComponentManager, zone string) controller.PhaseControl {
	return controller.NewRegisterEndpointComponent(man, v1alpha1.CloudPhoneComponentType,
		constants.ServiceNameCloudPhone, constants.ServiceTypeCloudPhone,
		man.GetCluster().Spec.CloudPhone.Service.NodePort, "")
}

type cloudPhoneOptions struct {
	v1alpha1.CloudPhoneCommonConfig

	option.CommonDBOptions
}

func (m *cloudPhoneManager) getConfigMap(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string) (*corev1.ConfigMap, bool, error) {
	opt := &cloudPhoneOptions{}
	if err := option.SetOptionsDefault(opt, constants.ServiceTypeCloudPhone); err != nil {
		return nil, false, err
	}
	config := cfg.CloudPhone

	switch oc.Spec.GetDbEngine(oc.Spec.CloudPhone.DbEngine) {
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

	opt.CloudPhoneCommonConfig = config.CloudPhoneCommonConfig

	opt.EnableAmdTestFilesMapping = true

	return m.newServiceConfigMap(v1alpha1.CloudPhoneComponentType, "", oc, opt), false, nil
}

func (m *cloudPhoneManager) getService(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string) []*corev1.Service {
	return []*corev1.Service{m.newSingleNodePortService(v1alpha1.CloudPhoneComponentType, oc, int32(oc.Spec.CloudPhone.Service.NodePort), int32(cfg.CloudPhone.Port))}
}

func (m *cloudPhoneManager) getDeployment(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string) (*apps.Deployment, error) {
	deploy, err := m.newCloudServiceSinglePortDeploymentWithReadinessProbePath(
		v1alpha1.CloudPhoneComponentType, "", oc, &oc.Spec.CloudPhone.DeploymentSpec,
		int32(cfg.CloudPhone.Port), true, false,
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

func (m *cloudPhoneManager) getDeploymentStatus(oc *v1alpha1.OnecloudCluster, zone string) *v1alpha1.DeploymentStatus {
	return &oc.Status.CloudPhone
}
