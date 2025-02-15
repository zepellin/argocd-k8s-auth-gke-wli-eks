# ArgoCD external cluster EKS config provider for GKE (workload identity)

The purpose of this application is to facilitate identity based (without use of any permanents credentials) authentication of EKS clusters in ArgoCD running on Google Kubernetes Engine (GKE) clusters with workload identity.

## Table of Contents

- [Introduction](#introduction)
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
  - [Installation](#installation)
  - [Usage](#usage)
- [ArgoCD Configuration](#argocd-configuration)
- [Contributing](#contributing)
- [License](#license)
- [Credits](#credits)
- [Additional resources](#additional-resources)

## Introduction

A scenario this application covers is an ArgoCD instance running on GKE and using workload identity and Google Cloud -> AWS IAM role federation to authenticate EKS clusters without need of providing any kind of long term credentials. The program uses GKE/GCE provided OAuth token to assume AWS role and generate pre-signed URL for EKS authentication.

## Prerequisites

1. A Google Cloud environment configured with IAM identity. This could be a VM instance using a service account identity or a GKE pod configured with GKE workload identity. In the case of ArgoCD this means having `argocd-server` and `argocd-application-controller` deployments using workload identity. Workload identity can be easily configured using the official workload identity [terraform module](https://registry.terraform.io/modules/terraform-google-modules/kubernetes-engine/google/latest/submodules/workload-identity).
2. An AWS role that is configured to trust the GCP service account used in the environment running the program. In the case of ArgoCD these are the service accounts used by the `argocd-server` and `argocd-application-controller` deployments/pods. This involves setting up AWS IAM role trust policy for `sts:AssumeRoleWithWebIdentity` action specifying `accounts.google.com` federated principal (more documentation [here](https://gist.github.com/wvanderdeijl/c6a9a9f26149cea86039b3608e3556c1)).
3. The IAM role from step 3. having appropriate permissions (policies attached) for EKS cluster(s) management.

## Getting started

### Installation

Download precompiled binary for your platform from the repository' [releases page](https://github.com/zepellin/argocd-k8s-auth-gke-wli-eks/releases). In the case of ArgoCD the binary has to be available in the `argocd-server` and `argocd-application-controller` deployments/pods.

The binary can be shipped via [custom ArgoCD images](https://argo-cd.readthedocs.io/en/stable/operator-manual/custom_tools/#byoi-build-your-own-image), or [added via volume mounts](https://argo-cd.readthedocs.io/en/stable/operator-manual/custom_tools/#adding-tools-via-volume-mounts) and placed in the `argocd-server` and `argocd-application-controller` deployments/pods.

Example for [ArgoCD official Helm Chart](https://github.com/argoproj/argo-helm/blob/main/charts/argo-cd/values.yaml#L655-L675):

```yaml
controller and server:
  ...
  initContainers:
   - name: download-tools
     image: alpine:3
     command: [sh, -c]
     args:
       - wget -qO k8s-auth-gke-wli-eks https://github.com/zepellin/argocd-k8s-auth-gke-wli-eks/releases/download/v0.1.0/k8s-auth-gke-wli-eks-v0.1.0-linux-amd64 && chmod +x k8s-auth-gke-wli-eks && mv k8s-auth-gke-wli-eks /argo-k8s-auth-gke-wli-eks/
     volumeMounts:
       - mountPath: /argo-k8s-auth-gke-wli-eks
         name: argo-k8s-auth-gke-wli-eks

  volumeMounts:
   - mountPath: /usr/local/bin/k8s-auth-gke-wli-eks
     name: argo-k8s-auth-gke-wli-eks
     subPath: k8s-auth-gke-wli-eks

  volumes:
   - name: argo-k8s-auth-gke-wli-eks
     emptyDir: {}
```

### Usage

The program takes following arguments:

- **-rolearn**: The AWS IAM role ARN to assume (required).
- **-cluster**: The name of the AWS EKS cluster for which you need credentials (required).
- **-stsregion**: AWS STS region to which requests are made (optional, default: us-east-1).

Example:

```bash
k8s-auth-gke-wli-eks -rolearn "arn:aws:iam::123456789012:role/argocdrole" -cluster "my-eks-cluster-name" -stsregion "us-east-1"
```

## ArgoCD Configuration

Create a secret defining secret in your ArgoCD namespace where `data.config` is base64 encoded section as in following example.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-eks-cluster-name-secret
  labels:
    argocd.argoproj.io/secret-type: cluster
type: Opaque
stringData:
  name: my-eks-cluster-name
  server: https://213456423213456789456123ABCDEF.grx.us-east-1.eks.amazonaws.com
  config: |
    {
      "execProviderConfig": {
        "command": "k8s-auth-gke-wli-eks",
        "args": [
            "-rolearn",
            "arn:aws:iam::123456789012:role/argocdrole",
            "-cluster",
            "my-eks-cluster-name",
            "-stsregion",
            "us-east-2"
        ],
        "apiVersion": "client.authentication.k8s.io/v1beta1",
        "installHint": "k8s-auth-gke-wli-eks missing"
      },
      "tlsClientConfig": {
        "insecure": false,
        "caData": "base64_encoded_ca_data"
      }
    }
```

## Features

The output of the program is an [ExecCredential](https://kubernetes.io/docs/reference/config-api/client-authentication.v1beta1/#client-authentication-k8s-io-v1beta1-ExecCredential) object of the [client.authentication.k8s.io/v1beta1](https://kubernetes.io/docs/reference/config-api/client-authentication.v1beta1/) Kubernetes API that is consumed by ArgoCD when authenticating EKS cluster.

## Contributing

If you'd like to contribute to this project, please follow the standard open-source contribution guidelines. Please report issues, submit feature requests, or create pull requests to improve the application.

## Additional resources

- Terraform GKE Worload identity module: [terraform-google-workload-identity](https://registry.terraform.io/modules/terraform-google-modules/kubernetes-engine/google/latest/submodules/workload-identity)
- Available keys for AWS web identity federation, for example of role trust with accounts.google.com. [AWS docs link](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_iam-condition-keys.html)
- How to use trust policies with IAM roles, for ways to futher secure AWS trust policies. [AWS Blog](https://aws.amazon.com/es/blogs/security/how-to-use-trust-policies-with-iam-roles/)

## Credits

- Use of aws-sdk-go-v2 to get working EKS token: [Github aws-sdk-go-v2](https://github.com/aws/aws-sdk-go-v2/issues/1922)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
