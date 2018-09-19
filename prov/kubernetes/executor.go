// Copyright 2018 Bull S.A.S. Atos Technologies - Bull, Rue Jean Jaures, B.P.68, 78340, Les Clayes-sous-Bois, France.
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

package kubernetes

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/ystia/yorc/config"
	"github.com/ystia/yorc/events"
	"github.com/ystia/yorc/helper/stringutil"
	"github.com/ystia/yorc/prov"
	"k8s.io/client-go/kubernetes"

	// Loading the gcp plugin to authenticate against GKE clusters
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type defaultExecutor struct {
	clientset *kubernetes.Clientset
}

func (e *defaultExecutor) ExecOperation(ctx context.Context, conf config.Configuration, taskID, deploymentID, nodeName string, operation prov.Operation) error {
	consulClient, err := conf.GetConsulClient()
	if err != nil {
		return err
	}

	logOptFields, ok := events.FromContext(ctx)
	if !ok {
		return errors.New("Missing contextual log optionnal fields")
	}
	logOptFields[events.NodeID] = nodeName
	logOptFields[events.OperationName] = stringutil.GetLastElement(operation.Name, ".")
	logOptFields[events.InterfaceName] = stringutil.GetAllExceptLastElement(operation.Name, ".")

	ctx = events.NewContext(ctx, logOptFields)

	kv := consulClient.KV()
	exec, err := newExecution(kv, conf, taskID, deploymentID, nodeName, operation)
	if err != nil {
		return err
	}

	if e.clientset == nil {
		e.clientset, err = initClientSet(conf)
		if err != nil {
			return err
		}
	}

	return exec.execute(ctx, e.clientset)
}

func initClientSet(cfg config.Configuration) (*kubernetes.Clientset, error) {
	kubConf := cfg.Infrastructures["kubernetes"]

	var conf *rest.Config
	var err error

	if kubConf == nil {
		conf, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, "Failed to build kubernetes InClusterConfig")
		}
	} else {
		kubeMasterIP := kubConf.GetString("master_url")
		kubeConfigPathOrContent := kubConf.GetString("kubeconfig")
		if kubeConfigPathOrContent == "" && kubeMasterIP == "" {
			return nil, errors.New(`Missing  mandatory parameter in the "kubernetes" infrastructure configuration, either kubeconfig or master_url must be defined`)
		}

		// kubeconfig yorc configuration parameter is either a path to a file
		// or a string providing the Kubernetes configuration details.
		// The kubernetes go API expects to get a file, creating it if necessary
		var kubeConfigPath string
		var wasPath bool
		if kubeConfigPathOrContent != "" {
			if kubeConfigPath, wasPath, err = stringutil.GetFilePath(kubeConfigPathOrContent); err != nil {
				return nil, err
			}
			if !wasPath {
				defer os.Remove(kubeConfigPath)
			}
		}

		// Google Application credentials are needed when attempting to access
		// Kubernetes Engine outside the Kubernetes cluster from a host where gcloud
		// is not installed.
		// The application_credentials yorc configuration parameter is either a
		// path to a file or a string providing the credentials.
		// Google API expects a path to a file to be provided in an environment
		// variable GOOGLE_APPLICATION_CREDENTIALS.
		// So creating a file if necessary

		applicationCredsPathOrContent := kubConf.GetString("application_credentials")
		if applicationCredsPathOrContent != "" {

			applicationCredsPath, wasPath, err := stringutil.GetFilePath(applicationCredsPathOrContent)
			if err != nil {
				return nil, err
			}
			if !wasPath {
				defer os.Remove(applicationCredsPath)
			}

			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", applicationCredsPath)
			defer os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		}

		conf, err = clientcmd.BuildConfigFromFlags(kubeMasterIP, kubeConfigPath)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to build kubernetes config")
		}

		if kubeConfigPath == "" {
			conf.TLSClientConfig.Insecure = kubConf.GetBool("insecure")
			conf.TLSClientConfig.CAFile = kubConf.GetString("ca_file")
			conf.TLSClientConfig.CertFile = kubConf.GetString("cert_file")
			conf.TLSClientConfig.KeyFile = kubConf.GetString("key_file")
		} else if conf.AuthProvider != nil && conf.AuthProvider.Name == "gcp" {
			// When application credentials are set, using these creds
			// and not attempting to rely on the local host gcloud command to
			// access tokens
			delete(conf.AuthProvider.Config, "cmd-path")
		}
	}

	clientset, err := kubernetes.NewForConfig(conf)
	return clientset, errors.Wrap(err, "Failed to create kubernetes clientset from config")
}
