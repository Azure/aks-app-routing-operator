# Change Log

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
