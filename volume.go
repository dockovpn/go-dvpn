/*
 * Copyright (c) 2024. Dockovpn Solutions OÜ
 */

package go_dvpn

import (
	"context"
	"fmt"
	"github.com/docker/docker/client"
)

type volumeType struct {
	dvpn         DvpnInterface
	dockerClient *client.Client
	volumeId     string
}

type VolumeRemoveHandle interface {
	Remove()
}

func NewVolumeRemoveHandle(dvpn DvpnInterface, dockerClient *client.Client, volumeId string) VolumeRemoveHandle {
	return &volumeType{
		dvpn:         dvpn,
		dockerClient: dockerClient,
		volumeId:     volumeId,
	}
}

func (v *volumeType) Remove() {
	cli := v.dockerClient
	ctx := context.Background()

	// we have to remove container first
	v.dvpn.Close()

	fmt.Printf("removing volume [%s]\n", v.volumeId)

	err := cli.VolumeRemove(ctx, v.volumeId, true)
	if err != nil {
		println(err.Error())
	}
}
