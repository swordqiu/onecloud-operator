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
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	common_options "yunion.io/x/onecloud/pkg/cloudcommon/options"
	"yunion.io/x/onecloud/pkg/mcclient"

	"yunion.io/x/onecloud-operator/pkg/apis/constants"
	"yunion.io/x/onecloud-operator/pkg/apis/onecloud/v1alpha1"
	"yunion.io/x/onecloud-operator/pkg/controller"
	"yunion.io/x/onecloud-operator/pkg/manager"
	"yunion.io/x/onecloud-operator/pkg/util/onecloud"
	"yunion.io/x/onecloud-operator/pkg/util/option"
)

type registerManager struct {
	*ComponentManager
}

func newRegisterManager(man *ComponentManager) manager.Manager {
	return &registerManager{man}
}

func (m *registerManager) getProductVersions() []v1alpha1.ProductVersion {
	return []v1alpha1.ProductVersion{
		v1alpha1.ProductVersionCMP,
	}
}

func (m *registerManager) GetComponentType() v1alpha1.ComponentType {
	return v1alpha1.RegisterComponentType
}

func (m *registerManager) IsDisabled(oc *v1alpha1.OnecloudCluster) bool {
	return oc.Spec.Register.Disable || !IsEnterpriseEdition(oc) || !isInProductVersion(m, oc)
}

func (m *registerManager) getDBEngine(oc *v1alpha1.OnecloudCluster) v1alpha1.TDBEngineType {
	return oc.Spec.GetDbEngine(oc.Spec.Register.DbEngine)
}

func (m *registerManager) getClickhouseConfig(cfg *v1alpha1.OnecloudClusterConfig) *v1alpha1.DBConfig {
	return nil
}

func (m *registerManager) Sync(oc *v1alpha1.OnecloudCluster) error {
	return syncComponent(m, oc, "")
}

func (m *registerManager) getDBConfig(cfg *v1alpha1.OnecloudClusterConfig) *v1alpha1.DBConfig {
	return &cfg.Register.DB
}

func (m *registerManager) getCloudUser(cfg *v1alpha1.OnecloudClusterConfig) *v1alpha1.CloudUser {
	return &cfg.Register.CloudUser
}

func (m *registerManager) getPhaseControl(man controller.ComponentManager, zone string) controller.PhaseControl {
	return controller.NewRegisterEndpointComponent(man, v1alpha1.RegisterComponentType,
		constants.ServiceNameRegister, constants.ServiceTypeRegister,
		constants.RegisterPort, "")
}

type registerOptions struct {
	common_options.CommonOptions
	common_options.DBOptions

	AccessKeyId     string `help:"aliyun access key id"`
	AccessKeySecret string `help:"aliyun access key secret"`

	// 用户注册验证码
	ShowCaptcha bool `help:"show captcha or not. " default:"true"`
}

func (m *registerManager) getConfigMap(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string) (*corev1.ConfigMap, bool, error) {
	opt := &registerOptions{}
	if err := option.SetOptionsDefault(opt, constants.ServiceTypeRegister); err != nil {
		return nil, false, err
	}
	config := cfg.Register
	option.SetMysqlOptions(&opt.DBOptions, oc.Spec.Mysql, config.DB)
	option.SetOptionsServiceTLS(&opt.BaseOptions, false)
	option.SetServiceCommonOptions(&opt.CommonOptions, oc, config.ServiceCommonOptions, cfg.CommonConfig)
	opt.Port = constants.RegisterPort

	return m.newServiceConfigMap(v1alpha1.RegisterComponentType, "", oc, opt), false, nil
}

func (m *registerManager) getService(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string) []*corev1.Service {
	return m.newSinglePortService(v1alpha1.RegisterComponentType, oc, oc.Spec.Register.Service.InternalOnly, int32(oc.Spec.Register.Service.NodePort), int32(cfg.Register.Port), oc.Spec.Register.SlaveReplicas > 0)
}

func (m *registerManager) getDeployment(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string) (*apps.Deployment, error) {
	return m.newCloudServiceSinglePortDeployment(v1alpha1.RegisterComponentType, "", oc, &oc.Spec.Register.DeploymentSpec, constants.RegisterPort, true, false)
}

func (m *registerManager) getDeploymentStatus(oc *v1alpha1.OnecloudCluster, zone string) *v1alpha1.DeploymentStatus {
	return &oc.Status.Register
}

func (m *registerManager) supportsReadOnlyService() bool {
	return false
}

func (m *registerManager) getReadonlyDeployment(oc *v1alpha1.OnecloudCluster, cfg *v1alpha1.OnecloudClusterConfig, zone string, deployment *apps.Deployment) *apps.Deployment {
	return nil
}

func (m *registerManager) getMcclientSyncFunc(oc *v1alpha1.OnecloudCluster) func(*mcclient.ClientSession) error {
	return func(s *mcclient.ClientSession) error {
		if m.IsDisabled(oc) {
			return onecloud.EnsureDisableService(s, constants.ServiceNameRegister)
		} else {
			return onecloud.EnsureEnableService(s, constants.ServiceNameRegister, false)
		}
	}
}
