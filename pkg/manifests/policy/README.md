# Policy

Policies in this directory are OPA policies we validate our App Routing resources against.

There are two kinds of manifests
- ConstraintTemplates. Define what the Constraint CRD will look like. We pull officially recommended ones by running `kustomize build | yq --split-exp '"./manifests/templates/" + .metadata.name + ".yaml"' --no-doc`. They are found in the `manifests/templates` directory.
- Constraints. Define how the rule will actually apply to manifests. We have to manually define these by implementing the ConstraintTemplates. After pulling any new ConstraintTemplates, if they're useful to our needs, we should implement them with a Constraint. Not all ConstraintTemplates should be implemented since some are more for organizational policy rather than generic best practices.

We need to periodically update the templates to pull any new best practices using the command above. The process should look like
1. Run the above kustomize command to pull any new ConstraintTemplates
2. Check the git diff. If no new Templates were added we can stop.
3. Look at the new Templates and decide if they would be useful to us. Are they related to best practices and security or are they more for organizational policy? If it's in the first category add a Constraint to the manifests directory implementing the Template.