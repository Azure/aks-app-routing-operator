// This Terraform configuration deploys a simple web application and Ingress setup to help test the Ingress controller 

# Namespace
resource "kubernetes_namespace" "ing-dev" {
  metadata {
    name = "ing-dev"
  }
}

# Deployment
resource "kubernetes_deployment" "aks-helloworld" {
  metadata {
    name      = "aks-helloworld"
    namespace = kubernetes_namespace.ing-dev.metadata[0].name
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = "aks-helloworld"
      }
    }

    template {
      metadata {
        labels = {
          app = "aks-helloworld"
        }
      }

      spec {
        container {
          name  = "aks-helloworld"
          image = "mcr.microsoft.com/azuredocs/aks-helloworld:v1"

          port {
            container_port = 80
          }

          env {
            name  = "TITLE"
            value = "Welcome to Azure Kubernetes Service (AKS)"
          }
        }
      }
    }
  }
}

# Service
resource "kubernetes_service" "aks_helloworld" {
  metadata {
    name      = "aks-helloworld"
    namespace = kubernetes_namespace.ing-dev.metadata[0].name
  }

  spec {
    selector = {
      app = "aks-helloworld"
    }

    port {
      port = 80
    }

    type = "ClusterIP"
  }
}

# Ingress
resource "kubernetes_ingress_v1" "aks_helloworld" {
  metadata {
    name      = "aks-helloworld"
    namespace = kubernetes_namespace.ing-dev.metadata[0].name
    annotations = {
      "nginx.ingress.kubernetes.io/rewrite-target" = "/$1"
    }
  }

  spec {
    ingress_class_name = "webapprouting.kubernetes.azure.com"

    rule {
      http {
        path {
          path      = "/hello"
          path_type = "Prefix"
          backend {
            service {
              name = kubernetes_service.aks_helloworld.metadata[0].name
              port {
                number = 80
              }
            }
          }
        }
      }
    }
  }
}
