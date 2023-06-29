package main

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"time"

	"github.com/fatih/color"
	goVersion "go.hein.dev/go-version"

	sthingsK8s "github.com/stuttgart-things/sthingsK8s"

	"github.com/redis/go-redis/v9"
	"github.com/stuttgart-things/redisqueue"
)

type AnsibleJobstruct struct {
	Name string
}

const banner = `
__  __  _____  ____         _____  ______  _____  ____  _    _ _____   _____ ______       _____ _____  ______       _______ ____  _____
|  \/  |/ ____|/ __ \       |  __ \|  ____|/ ____|/ __ \| |  | |  __ \ / ____|  ____|     / ____|  __ \|  ____|   /\|__   __/ __ \|  __ \
| \  / | (___ | |  | |______| |__) | |__  | (___ | |  | | |  | | |__) | |    | |__ ______| |    | |__) | |__     /  \  | | | |  | | |__) |
| |\/| |\___ \| |  | |______|  _  /|  __|  \___ \| |  | | |  | |  _  /| |    |  __|______| |    |  _  /|  __|   / /\ \ | | | |  | |  _  /
| |  | |____) | |__| |      | | \ \| |____ ____) | |__| | |__| | | \ \| |____| |____     | |____| | \ \| |____ / ____ \| | | |__| | | \ \
|_|  |_|_____/ \____/       |_|  \_|______|_____/ \____/ \____/|_|  \_\\_____|______|     \_____|_|  \_|______/_/    \_|_|  \____/|_|  \_\

`

var (
	version       = "unset"
	date          = "unknown"
	commit        = "unknown"
	output        = "yaml"
	redisServer   = os.Getenv("REDIS_SERVER")
	redisPort     = os.Getenv("REDIS_PORT")
	redisPassword = os.Getenv("REDIS_PASSWORD")
	redisStream   = "q9:1"
)

func main() {

	// Output banner + version output
	color.Cyan(banner)
	resp := goVersion.FuncWithOutput(false, version, commit, date, output)
	color.Magenta(resp + "\n")

	c, err := redisqueue.NewConsumerWithOptions(&redisqueue.ConsumerOptions{
		VisibilityTimeout: 60 * time.Second,
		BlockingTimeout:   5 * time.Second,
		ReclaimInterval:   1 * time.Second,
		BufferSize:        100,
		Concurrency:       10,
		RedisClient: redis.NewClient(&redis.Options{
			Addr:     redisServer + ":" + redisPort,
			Password: redisPassword,
			DB:       0,
		}),
	})

	if err != nil {
		panic(err)
	}

	c.Register(redisStream, processTasks)

	go func() {
		for err := range c.Errors {
			// handle errors accordingly
			fmt.Printf("err: %+v\n", err)
		}
	}()

	fmt.Println("POLLING FOR REDIS STREAM " + redisStream + " ON " + redisServer + ":" + redisPort)

	c.Run()

	fmt.Println("POLLING FOR REDIS STREAM STOPPED")
}

func processTasks(msg *redisqueue.Message) error {

	fmt.Println("SCANNING MESSAGE", msg.Values)

	fmt.Printf("name: %v\n", msg.Values["name"])

	job := AnsibleJobstruct{
		Name: msg.Values["name"].(string),
	}

	renderedTemplate := renderJobTemplate(job)
	fmt.Println(renderedTemplate)

	clusterConfig, _ := sthingsK8s.GetKubeConfig(os.Getenv("KUBECONFIG"))
	ns := sthingsK8s.GetK8sNamespaces(clusterConfig)

	fmt.Println("FOUND NAMESAPCES", ns)

	sthingsK8s.CreateDynamicResourcesFromTemplate(clusterConfig, []byte(renderedTemplate), "default")

	return nil
}

func renderJobTemplate(job AnsibleJobstruct) string {

	var buf bytes.Buffer

	tmpl, err := template.New("jobTemplate").Parse(ansibleJobTemplate)
	if err != nil {
		panic(err)
	}

	err = tmpl.Execute(&buf, job)
	if err != nil {
		panic(err)
	}

	return buf.String()
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
