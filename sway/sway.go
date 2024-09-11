// Copyright (c) The Amphitheatre Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sway

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildpacks/libcnb"
	"github.com/paketo-buildpacks/libpak"
	"github.com/paketo-buildpacks/libpak/bard"
	"github.com/paketo-buildpacks/libpak/crush"
	"github.com/paketo-buildpacks/libpak/effect"
	"github.com/paketo-buildpacks/libpak/sherpa"
)

type Sway struct {
	LayerContributor libpak.DependencyLayerContributor
	configResolver   libpak.ConfigurationResolver
	Logger           bard.Logger
	Executor         effect.Executor
}

func NewSway(dependency libpak.BuildpackDependency, cache libpak.DependencyCache, configResolver libpak.ConfigurationResolver) Sway {
	contributor := libpak.NewDependencyLayerContributor(dependency, cache, libcnb.LayerTypes{
		Cache:  true,
		Launch: true,
	})
	return Sway{
		LayerContributor: contributor,
		configResolver:   configResolver,
		Executor:         effect.NewExecutor(),
	}
}

func (r Sway) Contribute(layer libcnb.Layer) (libcnb.Layer, error) {
	r.LayerContributor.Logger = r.Logger
	return r.LayerContributor.Contribute(layer, func(artifact *os.File) (libcnb.Layer, error) {
		bin := filepath.Join(layer.Path, "bin")

		// TODO: May be use copy instead of it or update Extract Path or stripComponents=1
		r.Logger.Bodyf("Expanding %s to %s", artifact.Name(), bin)
		if err := crush.Extract(artifact, bin, 2); err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to expand %s\n%w", artifact.Name(), err)
		}

		// Must be set to executable
		file := filepath.Join(bin, PlanEntryForc)
		r.Logger.Bodyf("Setting %s as executable", file)
		if err := os.Chmod(file, 0755); err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to chmod %s\n%w", file, err)
		}

		// Must be set to PATH
		r.Logger.Bodyf("Setting %s in PATH", bin)
		if err := os.Setenv("PATH", sherpa.AppendToEnvVar("PATH", ":", bin)); err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to set $PATH\n%w", err)
		}

		// get version
		buf, err := r.Execute(PlanEntryForc, []string{"--version"})
		if err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to get %s version\n%w", PlanEntryForc, err)
		}
		version := strings.Split(strings.TrimSpace(buf.String()), " ")
		r.Logger.Bodyf("Checking %s version: %s", PlanEntryForc, version[1])

		fuelHome := filepath.Join(layer.Path, "fuel")

		// initialize wallet for deploy
		walletDir := filepath.Join(fuelHome, "wallets")
		if ok, err := r.InitializeWallet(walletDir); !ok {
			return libcnb.Layer{}, err
		}

		err = r.BuildContract()
		if err != nil {
			return libcnb.Layer{}, err
		}

		// need to set layer.Path/fuel to HOME variable
		forcGitHome := "/home/cnb/.forc"
		forcDependencyDir := filepath.Join(fuelHome, ".forc")
		copyDir(forcGitHome, forcDependencyDir)
		r.Logger.Bodyf("Copy dependency from %s to %s", forcGitHome, forcDependencyDir)

		layer.LaunchEnvironment.Default("WALLET_PATH", walletDir)
		layer.LaunchEnvironment.Default("FUEL_HOME", fuelHome)
		layer.LaunchEnvironment.Default("HOME", fuelHome)
		return layer, nil
	})
}

func (r Sway) Execute(command string, args []string) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	if err := r.Executor.Execute(effect.Execution{
		Command: command,
		Args:    args,
		Stdout:  buf,
		Stderr:  buf,
	}); err != nil {
		return buf, fmt.Errorf("%s: %w", buf.String(), err)
	}
	return buf, nil
}

func (r Sway) BuildProcessTypes(cr libpak.ConfigurationResolver, app libcnb.Application) ([]libcnb.Process, error) {
	processes := []libcnb.Process{}

	enableDeploy := cr.ResolveBool("BP_ENABLE_FORC_DEPLOY")
	if enableDeploy {
		processes = append(processes, libcnb.Process{
			Type:      PlanEntryForc,
			Command:   PlanEntryForc,
			Arguments: []string{"deploy", "--testnet", "--default-signer"},
			Default:   true,
		})
	}
	return processes, nil
}

func (r Sway) InitializeWallet(walletDir string) (bool, error) {
	r.Logger.Bodyf("Initializing deploy wallet and save to dir: %s", walletDir)
	os.MkdirAll(walletDir, os.ModePerm)

	// TODO: The official does not support command-line operations. For now, a wallet address is hardcoded. A patch is being submitted to the official team. Once accepted, this part will be optimized accordingly.
	wallet := `{"crypto":{"cipher":"aes-128-ctr","cipherparams":{"iv":"4d4a85c291ffad4477bd2c3e6e64f078"},"ciphertext":"89067d9bfffe1db17a37dd65c4e45e7953b8fc00143a6e1090937add1e3cb21153ca3156cc50e5ccedbe72534843bf7ad80fc4a9bc65fea82427e8905aa64fa241cb1522d8f5f817dfc144e45285a03ca36d4399f4276d0fd5a5c1d7d906ad2d7e5e861c7a58796ca08c73510fb5d806f15938b337f21e292d3eab92e25f1ed57a48906794cb2c6616220bde9bec526c5abc74a518ee6d92290105","kdf":"scrypt","kdfparams":{"dklen":32,"n":8192,"p":1,"r":8,"salt":"4a118a37f92754b0c472b9affc54a70f462d216b5e01a823c3e41ec234b96eba"},"mac":"132446b450cee5fe1c39f456c275cedeff72db61e84f9534de84ec462c745154"},"id":"380a7b4c-725e-4c3d-9667-50f9cdf43a94","version":3}`

	testnetWalletFile := filepath.Join(walletDir, ".wallet")
	os.WriteFile(testnetWalletFile, []byte(wallet), 0644)
	r.Logger.Bodyf("Initialize deploy wallet:%s success", testnetWalletFile)

	// args := []string{"wallet", "--path", testnetWalletFile, "accounts", "--unverified"}
	// if _, err := r.Execute(PlanEntryForc, args); err != nil {
	// 	return false, fmt.Errorf("unable to initialize deploy wallet\n%w", err)
	// }
	return true, nil
}

func (r Sway) BuildContract() error {
	args := []string{"build", "--release"}
	_, err := r.Execute(PlanEntryForc, args)
	if err != nil {
		return fmt.Errorf("unable to build contract\n%w", err)
	}
	r.Logger.Bodyf("Build contract success")
	return nil
}

func (r Sway) Name() string {
	return r.LayerContributor.LayerName()
}

// copyDir recursively copies a directory tree, attempting to preserve permissions.
// Source directory must exist, while destination directory must not.
func copyDir(src string, dst string) error {
	var err error
	var srcInfo os.FileInfo

	if srcInfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	fds, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, fd := range fds {
		srcFp := filepath.Join(src, fd.Name())
		dstFp := filepath.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = copyDir(srcFp, dstFp); err != nil {
				fmt.Println(err)
			}
		} else {
			if err = copyFile(srcFp, dstFp); err != nil {
				fmt.Println(err)
			}
		}
	}
	return nil
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}

	err = destination.Sync()
	if err != nil {
		return err
	}

	return nil
}
