// RAINBOND, Application Management Platform
// Copyright (C) 2014-2017 Goodrain Co., Ltd.

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version. For any non-GPL usage of Rainbond,
// one or multiple Commercial Licenses authorized by Goodrain Co., Ltd.
// must be obtained first.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package exector

import (
	"context"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/goodrain/rainbond/pkg/builder/sources"
	"github.com/goodrain/rainbond/pkg/event"
	"github.com/pquerna/ffjson/ffjson"
)

//ImageShareItem ImageShareItem
type ImageShareItem struct {
	Namespace      string `json:"namespace"`
	TenantName     string `json:"tenant_name"`
	ServiceID      string `json:"service_id"`
	ServiceAlias   string `json:"service_alias"`
	ImageName      string `json:"image_name"`
	LocalImageName string `json:"local_image_name"`
	ShareID        string `json:"share_id"`
	Logger         event.Logger
	ShareInfo      struct {
		ServiceKey string `json:"service_key" `
		AppVersion string `json:"app_version" `
		EventID    string `json:"event_id"`
		ShareUser  string `json:"share_user"`
		ShareScope string `json:"share_scope"`
		ImageInfo  struct {
			HubURL      string `json:"hub_url"`
			HubUser     string `json:"hub_user"`
			HubPassword string `json:"hub_password"`
			Namespace   string `json:"namespace"`
		} `json:"image_info,omitempty"`
	} `json:"share_info"`
	DockerClient *client.Client
	EtcdCli      *clientv3.Client
}

//NewImageShareItem 创建实体
func NewImageShareItem(in []byte, DockerClient *client.Client, EtcdCli *clientv3.Client) (*ImageShareItem, error) {
	var isi ImageShareItem
	if err := ffjson.Unmarshal(in, &isi); err != nil {
		return nil, err
	}
	eventID := isi.ShareInfo.EventID
	isi.Logger = event.GetManager().GetLogger(eventID)
	isi.DockerClient = DockerClient
	isi.EtcdCli = EtcdCli
	return &isi, nil
}

//ShareService ShareService
func (i *ImageShareItem) ShareService() error {
	_, err := sources.ImagePull(i.DockerClient, i.LocalImageName, types.ImagePullOptions{}, i.Logger, 3)
	if err != nil {
		logrus.Errorf("pull image %s error: %s", i.LocalImageName, err.Error())
		i.Logger.Error(fmt.Sprintf("拉取应用镜像: %s失败", i.LocalImageName), map[string]string{"step": "builder-exector", "status": "failure"})
		return err
	}
	if err := sources.ImageTag(i.DockerClient, i.LocalImageName, i.ImageName, i.Logger, 1); err != nil {
		logrus.Errorf("change image tag error: %s", err.Error())
		i.Logger.Error(fmt.Sprintf("修改镜像tag: %s -> %s 失败", i.LocalImageName, i.ImageName), map[string]string{"step": "builder-exector", "status": "failure"})
		return err
	}
	auth, err := sources.EncodeAuthToBase64(types.AuthConfig{Username: i.ShareInfo.ImageInfo.HubUser, Password: i.ShareInfo.ImageInfo.HubPassword})
	if err != nil {
		logrus.Errorf("make auth base63 push image error: %s", err.Error())
		i.Logger.Error(fmt.Sprintf("推送镜像内部错误"), map[string]string{"step": "builder-exector", "status": "failure"})
		return err
	}
	ipo := types.ImagePushOptions{
		RegistryAuth: auth,
	}
	err = sources.ImagePush(i.DockerClient, i.ImageName, ipo, i.Logger, 8)
	if err != nil {
		logrus.Errorf("push image into registry error: %s", err.Error())
		i.Logger.Error("推送镜像至镜像仓库失败", map[string]string{"step": "builder-exector", "status": "failure"})
		return err
	}
	return nil
}

//ShareStatus share status result
//ShareStatus share status result
type ShareStatus struct {
	ShareID string `json:"share_id,omitempty"`
	Status  string `json:"status,omitempty"`
}

func (s ShareStatus) String() string {
	b, _ := ffjson.Marshal(s)
	return string(b)
}

//UpdateShareStatus 更新任务执行结果
func (i *ImageShareItem) UpdateShareStatus(status string) error {
	var ss = ShareStatus{
		ShareID: i.ShareID,
		Status:  status,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := i.EtcdCli.Put(ctx, fmt.Sprintf("/rainbond/shareresult/%s", i.ShareID), ss.String())
	if err != nil {
		logrus.Errorf("put shareresult  %s into etcd error, %v", i.ShareID, err)
		i.Logger.Error("存储检测结果失败。", map[string]string{"step": "callback", "status": "failure"})
	}
	i.Logger.Info("创建检测结果成功。", map[string]string{"step": "last", "status": "success"})
	return nil
}