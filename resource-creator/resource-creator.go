package main

import (
	"fmt"
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

	clusterConfig, _ := sthingsK8s.GetKubeConfig(os.Getenv("KUBECONFIG"))
	ns := sthingsK8s.GetK8sNamespaces(clusterConfig)

	fmt.Println("FOUND NAMESAPCES", ns)

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

	c.Register(redisStream, process)

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

func process(msg *redisqueue.Message) error {
	fmt.Printf("processing message: %v\n", msg.Values["index"])

	fmt.Printf("name: %v\n", msg.Values["name"])

	// job := AnsibleJobstruct{
	// 	Name: "hello",
	// }

	// tmpl, err := template.New("pipelinerun").Parse(ansibleJobTemplate)
	// if err != nil {
	// 	panic(err)
	// }

	// var buf bytes.Buffer

	// err = tmpl.Execute(&buf, job)

	// if err != nil {
	// 	fmt.Println(err)
	// }

	// fmt.Println(buf.String())

	return nil
}
