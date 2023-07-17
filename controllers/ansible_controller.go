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

	"github.com/redis/go-redis/v9"

	"github.com/stuttgart-things/redisqueue"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	machineshopv1beta1 "github.com/stuttgart-things/machine-shop-operator/api/v1beta1"
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

	// GET VARIABLES FROM CR
	var (
		hosts    []string = ansibleCR.Spec.Hosts
		playbook string   = ansibleCR.Spec.Playbook
		vars     []string = ansibleCR.Spec.Vars
	)

	fmt.Println("REDIS_SERVER", os.Getenv("REDIS_SERVER")+":"+os.Getenv("REDIS_PORT"))
	fmt.Println(hosts)

	fmt.Println("playbook", playbook)
	fmt.Println(vars)

	// CREATE VALUES FOR INVENTORY
	inventory := createInventoryValues(hosts)
	fmt.Println(inventory)

	redisValues := map[string]interface{}{
		"template":                      "inventory.gotmpl",
		"name":                          "ansible-inventory",
		"namespace":                     "machine-shop",
		"all":                           "localhost",
		"loop-master":                   "rt.rancher.com;rt-2.rancher.com;rt-3.rancher.com",
		"loop-worker":                   "rt-4.rancher.com;rt-5.rancher.com",
		"merge-inventory;master;worker": "",
	}

	// ENQUEUE INVENTORY IN REDIS STREAMS
	fmt.Println("ENQUEUE INVENTORY IN REDIS STREAMS: ", os.Getenv("REDIS_STREAM"))

	p, err := redisqueue.NewProducerWithOptions(&redisqueue.ProducerOptions{
		MaxLen:               10000,
		ApproximateMaxLength: true,
		RedisClient: redis.NewClient(&redis.Options{
			Addr:     os.Getenv("REDIS_SERVER") + ":" + os.Getenv("REDIS_PORT"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       0,
		}),
	})
	if err != nil {
		panic(err)
	}

	redisStreamErr := p.Enqueue(&redisqueue.Message{
		Stream: os.Getenv("REDIS_STREAM"),
		Values: redisValues,
	})

	if redisStreamErr != nil {
		panic(err)
	}

	// for range time.Tick(time.Second * 10) {
	// 	if checkForAnsibleJob(playbook) {
	// 		break
	// 	}
	// }

	fmt.Println("ANSIBLE " + playbook + " PLAYBOOK-FINISHED!")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AnsibleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&machineshopv1beta1.Ansible{}).
		Complete(r)
}

func createInventoryValues(hostsGroups []string) (inventory map[string][]string) {

	inventory = make(map[string][]string)

	for _, groups := range hostsGroups {
		groups := strings.Split(groups, ":")
		hosts := strings.Split(strings.TrimSpace(groups[1]), ";")
		inventory[strings.TrimSpace(groups[0])] = strings.Split(strings.TrimSpace(groups[1]), ";")

		fmt.Println("GROUP:", strings.TrimSpace(groups[0]))
		fmt.Println("HOSTS:", hosts)
	}

	return
}
