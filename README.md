# Bridge and Compliance

Bridge and Compliance a services that lived in the [stellar/go](https://github.com/stellar/go) repository under the services directory but are now deprecated and have been deleted from that repository.

They were extracted from stellar/go at revision `698593a43c972293c084ce8ad0bea99b95b6dd9b` using the following commands:

```
git checkout -b bridge-compliance 698593a43c972293c084ce8ad0bea99b95b6dd9b
git filter-branch --prune-empty --subdirectory-filter services
git filter-branch -f --tree-filter 'find . ! \( -path "./bridge*" -o -path "./compliance*" -o -path "./internal*" -o -path "./.git*" -o -path "." \) -exec rm -fr {} +' --prune-empty HEAD

find . -type f -exec sed -i 's|github\.com/stellar/go/services/bridge|github.com/stellar-depreca
ted/bridge-compliance/bridge|' {} \;                                                                  
find . -type f -exec sed -i 's|github\.com/stellar/go/services/compliance|github.com/stellar-dep
recated/bridge-compliance/compliance|' {} \;
find . -type f -exec sed -i 's|github\.com/stellar/go/services/internal|github.com/stellar-depre
cated/bridge-compliance/internal|' {} \; 

git checkout 698593a43c972293c084ce8ad0bea99b95b6dd9b -- go.mod go.sum
go mod edit -module=github.com/stellar-deprecated/bridge-compliance
go mod edit -require=github.com/stellar/go@v0.0.0-20200427192452-698593a43c97
```
