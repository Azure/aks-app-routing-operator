# Change Log

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2023-01-11

### Changes

- Slightly lowered priority class to system cluster critical - [#148](https://github.com/Azure/aks-app-routing-operator/pull/148)

### Added

- Log for number of target Nginx replicas - [#144](https://github.com/Azure/aks-app-routing-operator/pull/144)
- CRD YAML definition-based validation and defaults - [#150](https://github.com/Azure/aks-app-routing-operator/pull/150)

### Removed

- Webhooks and webhook logic - [#151](https://github.com/Azure/aks-app-routing-operator/pull/151)

## [0.1.2] - 2023-11-28

### Changes

- Make DNS Zone resource group check case insensitive - [#137](https://github.com/Azure/aks-app-routing-operator/pull/137)
- Make Default NginxIngressController always reconcile - [#138](https://github.com/Azure/aks-app-routing-operator/pull/138)
- Expose useful function for interacting with NginxIngressController publicly - [#140](https://github.com/Azure/aks-app-routing-operator/pull/140), [#141](https://github.com/Azure/aks-app-routing-operator/pull/141), [#142](https://github.com/Azure/aks-app-routing-operator/pull/142)

## [0.1.1] - 2023-11-12

### Added

- Add lowercase validation to NginxIngressController CRD - [#136](https://github.com/Azure/aks-app-routing-operator/pull/136)

## [0.1.0] - 2023-11-11

### Added

- Add NginxIngressController CRD - [#121](https://github.com/Azure/aks-app-routing-operator/pull/121)
- Apply CRDs on operator startup - [#122](https://github.com/Azure/aks-app-routing-operator/pull/122)
- Add CRD Webhooks - [#123](https://github.com/Azure/aks-app-routing-operator/pull/123)
- Add prom metrics to Webhooks - [#125](https://github.com/Azure/aks-app-routing-operator/pull/125)
- Add CRD reconciler - [#124](https://github.com/Azure/aks-app-routing-operator/pull/124)
- Add default NginxIngressController reconciler - [#126](https://github.com/Azure/aks-app-routing-operator/pull/126)
- Switch all controllers to use CRD as source of truth - [#128](https://github.com/Azure/aks-app-routing-operator/pull/128)
- Add new Ingress events - [#130](https://github.com/Azure/aks-app-routing-operator/pull/130)
- Add ownership reference to webhook config - [#129](https://github.com/Azure/aks-app-routing-operator/pull/129)
- Generate Webhook certs - [#131](https://github.com/Azure/aks-app-routing-operator/pull/131)
- Use dns hostname for cert - [#133](https://github.com/Azure/aks-app-routing-operator/pull/133)
- Add E2e tests for CRD - [#132](https://github.com/Azure/aks-app-routing-operator/pull/132)

## [0.0.7] - 2023-11-04

### Changed

- Revert label selector on Placeholder pod to be backwards compatible

## [0.0.6] - 2023-10-27

### Added

- Removed resource limtis and add topology spread - [#119](https://github.com/Azure/aks-app-routing-operator/pull/119)
- Add managed-by label to all managed resource and check for that before cleaning - [#111](https://github.com/Azure/aks-app-routing-operator/pull/111)

### Changed

- Upgrade NGINX Ingress Controller to v1.9.4 - [#118](https://github.com/Azure/aks-app-routing-operator/pull/118)

## [0.0.5] - 2023-10-13

### Added

- Improved logging across entire operator - [#110](https://github.com/Azure/aks-app-routing-operator/pull/110)

### Changed

- Upgrade NGINX Ingress Controller to v1.8.4 - [#113](https://github.com/Azure/aks-app-routing-operator/pull/113)

## [0.0.4] - 2023-10-05

### Added

- Improved error logging - [#97](https://github.com/Azure/aks-app-routing-operator/pull/97)
- Improved E2E testing framework that tests upgrade story and all operator configurations - [#79](https://github.com/Azure/aks-app-routing-operator/pull/79), [#90](https://github.com/Azure/aks-app-routing-operator/pull/90), [#95](https://github.com/Azure/aks-app-routing-operator/pull/95), [#98](https://github.com/Azure/aks-app-routing-operator/pull/98), [#100](https://github.com/Azure/aks-app-routing-operator/pull/100), [#104](https://github.com/Azure/aks-app-routing-operator/pull/104)

### Changed

- Bump dependencies - [#92](https://github.com/Azure/aks-app-routing-operator/pull/92)
- Upgrade NGINX Ingress Controller to v1.8.1 - [#89](https://github.com/Azure/aks-app-routing-operator/pull/89)

## [0.0.3] - 2023-08-25

### Added

- Leader election and operator multi-replica support (adds resilliency through multliple replicas and PDB) - [#64](https://github.com/Azure/aks-app-routing-operator/pull/64)
- Switch Operator logging to Zap for JSON structured logging - [#69](https://github.com/Azure/aks-app-routing-operator/pull/69)
- Add filename and line numbers to logs - [#72](https://github.com/Azure/aks-app-routing-operator/pull/72)
- Add Unit test coverage checks and validation to repository - [#74](https://github.com/Azure/aks-app-routing-operator/pull/74)
- Add CodeQL security scanning to repository - [#70](https://github.com/Azure/aks-app-routing-operator/pull/70)
- Add Prometheus metrics for reconciliation loops total and errors - [#76](https://github.com/Azure/aks-app-routing-operator/pull/76)
- Increase unit test coverage - [#77](https://github.com/Azure/aks-app-routing-operator/pull/77), [#82](https://github.com/Azure/aks-app-routing-operator/pull/82)
- Add controller name structure to each controller so logs and metrics have consistient and related controller names - [#80](https://github.com/Azure/aks-app-routing-operator/pull/80), [#84](https://github.com/Azure/aks-app-routing-operator/pull/84), [#85](https://github.com/Azure/aks-app-routing-operator/pull/85), [#86](https://github.com/Azure/aks-app-routing-operator/pull/86)

## [0.0.2] - 2023-07-10

### Fixed

- IngressClass Controller field immutable bug

## [0.0.1] - 2022-05-24

### Added

- Initial release of App Routing 🚢
