/*
Copyright 2023 patrick hermann.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	sthingsCli "github.com/stuttgart-things/sthingsCli"

	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	machineshopv1beta1 "github.com/stuttgart-things/machine-shop-operator/api/v1beta1"
)

var (
	templates = map[string]string{
		"inventory": "inventory.gotmpl",
		"playbook":  "playbook.gotmpl",
		"job":       "job.gotmpl",
	}
	kinds = map[string]string{
		"inventory": "ConfigMap",
		"playbook":  "ConfigMap",
		"job":       "job",
	}
	maxResourceCheckRetries = 10
	ansibleJobNamespace     = os.Getenv("ANSIBLE_JOB_NAMESPACE")
	redisStream             = os.Getenv("REDIS_STREAM")
	allStreamValues         []interface{}
	inventoryStreamValues   = make(map[string]interface{})
	playbookStreamValues    = make(map[string]interface{})
)

// AnsibleReconciler reconciles a Ansible object
type AnsibleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=machineshop.sthings.tiab.ssc.sva.de,resources=ansibles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=machineshop.sthings.tiab.ssc.sva.de,resources=ansibles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=machineshop.sthings.tiab.ssc.sva.de,resources=ansibles/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ansible object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *AnsibleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	log := ctrllog.FromContext(ctx)
	fmt.Println("REDIS_SERVER", os.Getenv("REDIS_SERVER")+":"+os.Getenv("REDIS_PORT"))

	log.Info("⚡️ Event received! ⚡️")
	log.Info("Request: ", "req", req)

	// VERIFY CR
	ansibleCR := &machineshopv1beta1.Ansible{}
	err := r.Get(ctx, req.NamespacedName, ansibleCR)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Ansible resource not found...")
		} else {
			log.Info("Error", err)
		}
	}

	// GET VARIABLES FROM CR + ENV
	var (
		hosts    []string = ansibleCR.Spec.Hosts
		playbook string   = ansibleCR.Spec.Playbook
		vars     []string = ansibleCR.Spec.Vars
	)

	// INVENTORY VALUES
	inventoryStreamValues["template"] = templates["inventory"]
	inventoryStreamValues["name"] = req.Name + "-inv"
	inventoryStreamValues["namespace"] = ansibleJobNamespace
	inventoryStreamValues["kind"] = kinds["inventory"]

	//CREATE VALUES FOR INVENTORY
	for _, groups := range hosts {
		fmt.Println(groups)
		groupName, hosts := createCrListVars(groups)
		inventoryStreamValues[groupName] = hosts
	}

	// PLAYBOOK VALUES
	playbookStreamValues["template"] = templates["playbook"]
	playbookStreamValues["playbook"] = playbook
	playbookStreamValues["name"] = req.Name + "-play"
	playbookStreamValues["namespace"] = ansibleJobNamespace
	playbookStreamValues["kind"] = kinds["playbook"]

	//CREATE VALUES FOR PLAYBOOK VARS
	for _, varNames := range vars {
		varName, value := createCrListVars(varNames)
		playbookStreamValues[varName] = value
	}

	allStreamValues = append(allStreamValues, inventoryStreamValues)
	allStreamValues = append(allStreamValues, playbookStreamValues)

	for _, values := range allStreamValues {
		streamValues, _ := values.(map[string]interface{})
		fmt.Println("ENQUEUING", streamValues["name"])

		if sthingsCli.EnqueueDataInRedisStreams(os.Getenv("REDIS_SERVER")+":"+os.Getenv("REDIS_PORT"), os.Getenv("REDIS_PASSWORD"), os.Getenv("REDIS_STREAM"), streamValues) {
			fmt.Println("⚡️ VALUES ENQUEUE IN REDIS STREAM ⚡️ " + redisStream)
		}

		// CHECK FOR VALUES IN REDIS
		try := 0
		for range time.Tick(time.Second * 5) {

			if try <= maxResourceCheckRetries {

				try++
				if sthingsCli.CheckRedisKV(os.Getenv("REDIS_SERVER")+":"+os.Getenv("REDIS_PORT"), os.Getenv("REDIS_PASSWORD"), kinds["inventory"]+"-"+streamValues["name"].(string), "created") {

					fmt.Println(streamValues["name"], "created")
					break
				}

			} else {
				fmt.Println("retries are exhausted..exiting")
				break
			}

		}
	}

	fmt.Println("ANSIBLE " + playbook + " PLAYBOOK-FINISHED!")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnsibleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&machineshopv1beta1.Ansible{}).
		Complete(r)
}

func createCrListVars(groups string) (varName string, values string) {

	group := strings.Split(groups, ":")
	varName = strings.TrimSpace(group[0])
	values = group[1]

	return
}
