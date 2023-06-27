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
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	machineshopv1beta1 "github.com/stuttgart-things/machine-shop-operator/api/v1beta1"
)

type AnsibleJobstruct struct {
	Name string
}

const ansibleJobTemplate = `
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ .Name }}
  namespace: machine-shop
  labels:
    app: machine-shop-operator
    machine-shop-operator: ansible
spec:
  template:
    metadata:
      name: 2023-06-27-configure-rke-node-mary
      labels:
        app: machine-shop-operator
        machine-shop-operator: ansible
    spec:
      containers:
        - name: manager
          image: eu.gcr.io/stuttgart-things/sthings-ansible:8.0.0-4
          imagePullPolicy: Always
          securityContext:
            allowPrivilegeEscalation: true
            privileged: true
            runAsNonRoot: true
            readOnlyRootFilesystem: false
            runAsUser: 65532
          env:
            - name: ANSIBLE_HOST_KEY_CHECKING
              value: "False"
            - name: INV_PATH
              value: "/tmp/inv"
            - name: TARGETS
              value: "mso-vm2.tiab.labda.sva.de"
          envFrom:
            - secretRef:
                name: vault
          resources:
            requests:
              cpu: 10m
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 768Mi
          command:
            - /bin/sh
            - -ec
            - touch ${INV_PATH} && ansible-playbook -i $INV_PATH $HOME/ansible/play.yaml -vv -e prepare_env=true -e execute_baseos=true -e target_play=configure-rke-node
          volumeMounts:
            - name: ansible
              mountPath: /home/nonroot/ansible
      restartPolicy: Never
      volumes:
        - name: ansible
          configMap:
            name: ansible
`

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
		hosts    string   = ansibleCR.Spec.Hosts
		playbook string   = ansibleCR.Spec.Playbook
		vars     []string = ansibleCR.Spec.Vars
	)
	fmt.Println("REDIS_SERVER", os.Getenv("REDIS_SERVER")+":"+os.Getenv("REDIS_PORT"))
	fmt.Println("REDIS_PASSWORD", os.Getenv("REDIS_PASSWORD"))

	fmt.Println("hosts:", hosts)
	fmt.Println("playbook:", playbook)
	fmt.Println("vars:", vars)

	for range time.Tick(time.Second * 10) {
		if checkForAnsibleJob(playbook) {
			break
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

func checkForAnsibleJob(name string) (jobIsFinished bool) {

	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_SERVER") + ":" + os.Getenv("REDIS_PORT"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// TEST RENDER JOB
	renderedJob := renderAnsibleJob("base-os")
	fmt.Println(renderedJob)

	// CHECK IF KEY EXISTS IN REDIS
	fmt.Println("CHECKING IF KEY " + name + " EXISTS..")
	keyExists, err := rdb.Exists(context.TODO(), name).Result()
	if err != nil {
		panic(err)
	}

	// CHECK FOR VALUE/STATUS IN REDIS
	if keyExists == 1 {

		fmt.Println("KEY " + name + " EXISTS..CHECKING FOR IT'S VALUE")

		jobsStatus, err := rdb.Get(context.TODO(), name).Result()
		if err != nil {
			panic(err)
		}

		if jobsStatus == "finished" {
			jobIsFinished = true
		}

		fmt.Println("STATUS", jobsStatus)

	} else {
		fmt.Println("KEY " + name + " DOES NOT EXIST)")
	}

	return
}

func renderAnsibleJob(name string) string {

	job := AnsibleJobstruct{
		Name: name,
	}

	tmpl, err := template.New("pipelinerun").Parse(ansibleJobTemplate)
	if err != nil {
		panic(err)
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, job)

	if err != nil {
		fmt.Println(err)
	}

	return buf.String()
}
