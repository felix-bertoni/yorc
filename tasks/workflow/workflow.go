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

package workflow

import (
	"fmt"
	"path"

	"github.com/hashicorp/consul/api"
	"github.com/satori/go.uuid"

	"github.com/ystia/yorc/helper/consulutil"
	"github.com/ystia/yorc/log"
	"github.com/ystia/yorc/tasks/workflow/builder"
)

// createWorkflowStepsOperations returns Consul transactional KV operations for initiating workflow execution
func createWorkflowStepsOperations(taskID string, steps []*step) api.KVTxnOps {
	ops := make(api.KVTxnOps, 0)
	var stepOps api.KVTxnOps
	for _, step := range steps {
		// Add execution key for initial steps only
		execID := fmt.Sprint(uuid.NewV4())
		log.Debugf("Register task execution with ID:%q, taskID:%q and step:%q", execID, taskID, step.Name)
		stepExecPath := path.Join(consulutil.ExecutionsTaskPrefix, execID)
		stepOps = api.KVTxnOps{
			&api.KVTxnOp{
				Verb:  api.KVSet,
				Key:   path.Join(stepExecPath, "taskID"),
				Value: []byte(taskID),
			},
			&api.KVTxnOp{
				Verb:  api.KVSet,
				Key:   path.Join(stepExecPath, "step"),
				Value: []byte(step.Name),
			},
		}
		ops = append(ops, stepOps...)
	}
	return ops
}

func getCallOperationsFromStep(s *step) []string {
	ops := make([]string, 0)
	for _, a := range s.Activities {
		if a.Type() == builder.ActivityTypeCallOperation {
			ops = append(ops, a.Value())
		}
	}
	return ops
}
