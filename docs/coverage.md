# Coverage

Our PR gates include coverage tests ensuring that coverage is above a certain percentage.

Not everything is unit testable so we have a mechanism for excluding files. This should be used extremely sparingly, most code is unit testable and if something isn't think about how it could be refactored to be better tested.

To add a file or directory to be excluded simply append its Go representation to [.covignore](../.covignore).