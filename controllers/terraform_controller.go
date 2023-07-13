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

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/terraform-exec/tfexec"
	sthingsBase "github.com/stuttgart-things/sthingsBase"
	sthingsCli "github.com/stuttgart-things/sthingsCli"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	machineshopv1beta1 "github.com/stuttgart-things/machine-shop-operator/api/v1beta1"
)

// TerraformReconciler reconciles a Terraform object
type TerraformReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const regexPatternVaultSecretPath = `.+/data/.+:.+`

//+kubebuilder:rbac:groups=machineshop.sthings.tiab.ssc.sva.de,resources=terraforms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=machineshop.sthings.tiab.ssc.sva.de,resources=terraforms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=machineshop.sthings.tiab.ssc.sva.de,resources=terraforms/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Terraform object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *TerraformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	log := ctrllog.FromContext(ctx)
	log.Info("⚡️ Event received! ⚡️")
	log.Info("Request: ", "req", req)

	terraformCR := &machineshopv1beta1.Terraform{}
	err := r.Get(ctx, req.NamespacedName, terraformCR)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Terraform resource not found...")
		} else {
			log.Info("Error", err)
		}
	}

	// GET VARIABLES FROM CR
	var (
		tfVersion     string   = terraformCR.Spec.TerraformVersion
		template      string   = terraformCR.Spec.Template
		module        []string = terraformCR.Spec.Module
		backend       []string = terraformCR.Spec.Backend
		secrets       []string = terraformCR.Spec.Secrets
		variables     []string = terraformCR.Spec.Variables
		tfInitOptions []tfexec.InitOption
		applyOptions  []tfexec.ApplyOption
	)

	// WORKING DIRS
	var (
		logfilePath       = "/tmp/" + req.Name + ".log"
		workingDir        = "/tmp/tf/" + req.Name + "/"
		msTeamswebhookUrl = "https://365sva.webhook.office.com/webhookb2/2f14a9f8-4736-46dd-9c8c-31547ec37180@0a65cb1e-37d5-41ff-980a-647d9d0e4f0b/IncomingWebhook/a993544595464ce6af4f2f0461d55a17/dc3a27ed-396c-40b7-a9b2-f1a2b6b44efe"
	)

	// GET MODULE PARAMETER
	moduleParameter := make(map[string]interface{})
	for _, s := range module {
		keyValue := strings.Split(s, "=")
		moduleParameter[keyValue[0]] = keyValue[1]
	}

	// CHECK FOR VAULT ENV VARS
	vaultAuthType, vaultAuthFound := sthingsCli.VerifyVaultEnvVars()
	log.Info("⚡️ VAULT CREDENDITALS ⚡️", vaultAuthType, vaultAuthFound)

	if vaultAuthType == "approle" {
		client, err := sthingsCli.CreateVaultClient()

		if err != nil {
			log.Error(err, "token creation (by approle) not working")
		}

		token, err := client.GetVaultTokenFromAppRole()

		if err != nil {
			log.Error(err, "token creation (by approle) not working")
		}

		os.Setenv("VAULT_TOKEN", token)
	}

	// CONVERT ALL EXISTING SECRETS IN BACKEND+MODULE PARAMETERS
	backend = convertVaultSecretsInParameters(backend)
	secrets = convertVaultSecretsInParameters(secrets)

	// PRINT OUT CR
	fmt.Println("CR-NAME", req.Name)

	// READ + RENDER TF MODULE TEMPLATE
	moduleCallTemplate := sthingsBase.ReadFileToVariable("terraform/" + template)
	log.Info("⚡️ Rendering tf config ⚡️")
	renderedModuleCall, _ := sthingsBase.RenderTemplateInline(string(moduleCallTemplate), "missingkey=zero", "{{", "}}", moduleParameter)

	// CREATE TF FILES
	log.Info("⚡️ CREATING WORKING DIR AND PROJECT FILES ⚡️")
	sthingsBase.CreateNestedDirectoryStructure(workingDir, 0777)
	sthingsBase.StoreVariableInFile(workingDir+req.Name+".tf", string(renderedModuleCall))
	sthingsBase.StoreVariableInFile(workingDir+"terraform.tfvars", strings.Join(variables, "\n"))

	// TERRAFORM INIT
	tf := initalizeTerraform(workingDir, tfVersion)
	log.Info("⚡️ INITALIZE TERRAFORM ⚡️")
	tfInitOptions = append(tfInitOptions, tfexec.Upgrade(true))

	for _, backendParameter := range backend {
		tfInitOptions = append(tfInitOptions, tfexec.BackendConfig(strings.TrimSpace(backendParameter)))
	}

	err = tf.Init(context.Background(), tfInitOptions...)

	if err != nil {
		log.Info("ERROR RUNNING INIT")
		fmt.Print(err)
	}

	log.Info("⚡️ INITALIZING OF TERRAFORM DONE ⚡️")

	// TERRAFORM APPLY
	log.Info("⚡️ APPLYING.. ⚡️")

	for _, secret := range secrets {
		applyOptions = append(applyOptions, tfexec.Var(strings.TrimSpace(secret)))
	}

	// LOGFILE HANDLING
	logFileExists, _ := sthingsBase.VerifyIfFileOrDirExists(logfilePath, "file")
	if logFileExists {
		sthingsBase.DeleteFile(logfilePath)
	}

	fileWriter := sthingsBase.CreateFileLogger(logfilePath)
	tf.SetStdout(fileWriter)
	tf.SetStderr(fileWriter)

	err = tf.Apply(context.Background(), applyOptions...)

	if err != nil {
		// fmt.Println("ERROR RUNNING APPLY: %s", err)
		log.Error(err, "TF APPLY ABORTED!")
	}

	log.Info("TF APPLY DONE!")

	// EXTRACT LOGGING INFORMATION
	logfileApplyOperation := sthingsBase.ReadFileToVariable(logfilePath)
	// fmt.Println(logfileApplyOperation)

	applyStatus, _ := sthingsBase.GetRegexSubMatch(logfileApplyOperation, `(.*(?:Apply complete).*)`)
	log.Info("TERRAFORM-STATUS: " + applyStatus)

	var outputInformation string

	if len(sthingsBase.GetAllRegexMatches(logfileApplyOperation, `Outputs:`)) > 0 {
		s := strings.Split(logfileApplyOperation, "Outputs:")
		fmt.Println("outputInformation:")
		outputInformation, _ = sthingsBase.GetRegexSubMatch(s[1], `\[([^\[\]]*)\]`)
		outputInformationWithoutComma := strings.Replace(outputInformation, ",", "", -1)
		outputInformationWithoutQuotes := strings.Replace(outputInformationWithoutComma, "\"", "", -1)
		outputInformation = outputInformationWithoutQuotes
		log.Info("TERRAFORM-OUTPUTS: " + outputInformation)
	}

	webhook := sthingsCli.MsTeamsWebhook{Title: "stuttgart-things/machine-shop-operator", Text: req.Name + " was created \n" + applyStatus + "\n\n" + outputInformation, Color: "#DF813D", Url: msTeamswebhookUrl}

	sthingsCli.SendWebhookToTeams(webhook)
	log.Info("WEBHOOK SENDED")

	fmt.Println("FOO:", os.Getenv("WEBHOOK_URL"))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TerraformReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&machineshopv1beta1.Terraform{}).
		Complete(r)
}

func initalizeTerraform(terraformDir, terraformVersion string) (tf *tfexec.Terraform) {

	installer := &releases.ExactVersion{
		Product: product.Terraform,
		Version: version.Must(version.NewVersion(terraformVersion)),
	}

	execPath, err := installer.Install(context.Background())
	if err != nil {
		fmt.Println("Error installing Terraform")
		fmt.Print(err)
	}

	tf, err = tfexec.NewTerraform(terraformDir, execPath)
	if err != nil {
		fmt.Println("Error running Terraform")
		fmt.Print(err)
	}

	return

}

func convertVaultSecretsInParameters(parameters []string) (updatedParameters []string) {

	for _, parameter := range parameters {

		kvParameter := strings.Split(parameter, "=")
		updatedParameter := parameter

		if len(sthingsBase.GetAllRegexMatches(kvParameter[1], regexPatternVaultSecretPath)) > 0 {
			secretValue := sthingsCli.GetVaultSecretValue(kvParameter[1], os.Getenv("VAULT_TOKEN"))
			updatedParameter = kvParameter[0] + "=" + secretValue
		}

		updatedParameters = append(updatedParameters, updatedParameter)

	}

	return
}
