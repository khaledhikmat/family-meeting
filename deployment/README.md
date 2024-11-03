
Before you run Terraform:

- Create a storage bucket `family-meeting-generic` to store Terraform state

- Login to the Google cloud:

```bash
gcloud auth application-default login
```

- Add permission to allow the creation of service accounts:

```bash
gcloud projects add-iam-policy-binding family-meeting-aa853 --member="user:khaled.hikmat@gmail.com" --role="roles/iam.serviceAccountAdmin"
```

- Install plugin on kubectl. Follow [https://cloud.google.com/kubernetes-engine/docs/how-to/cluster-access-for-kubectl#install_plugin](https://cloud.google.com/kubernetes-engine/docs/how-to/cluster-access-for-kubectl#install_plugin)

- Get Kubectl context:

```bash
gcloud container clusters get-credentials primary --zone us-central1-a 
```

- Deploy Validation:

```bash
# This should be deployed only once!!!
kubectl apply -f k8s/oidc/service-account.yaml
kubectl get serviceaccounts
kubectl get nodes
kubectl apply -f k8s/nginx/deployment-with-init-gs.yaml
kubectl get deployment
kubectl get pods
kubectl get logs <pod> -c cloud-sdk
kubectl get logs <pod> -c nginx
kubectl apply -f k8s/nginx/public-lb.yaml
open http://104.197.180.76
kubectl delete svc <name>
```

## Anomalies

*Mostly, I see inconsistencies in creating and destroying via Terraform. It has been actually difficult to work within GKS.*

- Error on destroy:

```
Error: Error when reading or editing Project Service family-meeting-aa853/compute.googleapis.com: Error disabling service "compute.googleapis.com" for project "family-meeting-aa853": googleapi: Error 400: The service compute.googleapis.com is depended on by the following active service(s): container.googleapis.com; Please specify disable_dependent_services=true if you want to proceed with disabling all services.
│ Help Token: AYJSUtl2_WVXvSmVhHzKURFH-zp2Mnp6f8vnKhfKUHgMWx5R1YKm33I8khLmuYwpz-pYVTo9J6iyvTLZTuFIcayJACWFhQx63rI-mZ-avZhQZhjk
│ Details:
│ [
│   {
│     "@type": "type.googleapis.com/google.rpc.PreconditionFailure",
│     "violations": [
│       {
│         "subject": "?error_code=100001\u0026service_name=compute.googleapis.com\u0026services=container.googleapis.com",
│         "type": "googleapis.com"
│       }
│     ]
│   },
│   {
│     "@type": "type.googleapis.com/google.rpc.ErrorInfo",
│     "domain": "serviceusage.googleapis.com",
│     "metadata": {
│       "service_name": "compute.googleapis.com",
│       "services": "container.googleapis.com"
│     },
│     "reason": "COMMON_SU_SERVICE_HAS_DEPENDENT_SERVICES"
│   }
│ ]
│ , failedPrecondition
```

- Another error on destroy:

```
Error: Error when reading or editing Subnetwork: Delete "https://compute.googleapis.com/compute/v1/projects/family-meeting-aa853/regions/us-central1/subnetworks/private?alt=json": dial tcp [2607:f8b0:4023:1009::5f]:443: connect: connection refused
```

- `service-a` is not deleted when I destroy via Terraform. So next time I create resources, I get this error:

```
Error: Error creating service account: googleapi: Error 409: Service account service-a already exists within project projects/family-meeting-aa853.
│ Details:
│ [
│   {
│     "@type": "type.googleapis.com/google.rpc.ResourceInfo",
│     "resourceName": "projects/family-meeting-aa853/serviceAccounts/service-a@family-meeting-aa853.iam.gserviceaccount.com"
│   }
│ ]
│ , alreadyExists
│ 
│   with google_service_account.service-a,
│   on 9-service-account.tf line 2, in resource "google_service_account" "service-a":
│    2: resource "google_service_account" "service-a" {
```
