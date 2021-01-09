# opalego

Make Open Policy Agent more easily to integrate into golang application to implement RBAC.

## Per User/Group Policy Model

Oftentimes, we need not only checking user permissions but also need querying user abilities from
the RBAC system. That is, it needs to be able to return values other than true/false.

Although OPA Rego has that ability, it will become quite hard to read and write the Rego files.

While using per user/group rego model, it will be much simpler, for example: 

```rego
package user1

allowed_options[option] { option = "OPTION_1" }
allowed { allowed_options[input.option] }
```

```rego
package user2

allowed_options[option] { option = "OPTION_2" }
allowed_options[option] { option = "OPTION_3" }
allowed { allowed_options[input.option] }
```

In this way, we can avoid messing up shared rego rules and also avoid putting complexity
into external data, because everyone or group has their own rego.

## Generate Rego bundle from the external User/Group <-> Role relationship

 ```golang
package main

import (
	"fmt"
	"context"
	"github.com/rueian/opalego/pkg/bundle"
	"github.com/rueian/opalego/pkg/lego"
)

func main() {
	generator := lego.NewLego(bundle.Factory{
		Base: bundle.Base {
			Rego: `allowed { allowed_options[input.option] }`,
		},
		RegoPiece: map[string]string{
			"role1": `allowed_options[option] { option = "OPTION_1" }`,
			"role2": `allowed_options[option] { option = "OPTION_2" }`,
			"role3": `allowed_options[option] { option = "OPTION_3" }`,
		},
	})
	
	// When SetBundle is called, the rego bundle is generated,
	// it is safe to be called whenever external User/Group <-> Role relationship chagned.
	generator.SetBundle(bundle.Service{
		Members: map[string]bundle.Member{
			"user1": {Roles: []string{"role1"}},
			"user2": {Roles: []string{"role2", "role3"}},
        },
    })
	
	ctx := context.Background()
	fmt.Println(generator.Client().Query(ctx, lego.QueryOption{
		UID:   "user2",
		Rule:  "allowed",
		Input: map[string]interface{}{"option": "OPTION_2"},
	})) // return: true
	
	fmt.Println(generator.Client().Query(ctx, lego.QueryOption{
		UID:   "user1",
		Rule:  "allowed_options",
	})) // return: ["OPTION_1"]
}
 ```

## Easy to connect to OPA server sidecar on production

While opalego wrapping around the Golang OPA SDK to make application more easily to run, and to test,
it is recommended running OPA server sidecar to collect metrics or audit logs on production.

```golang
generator := lego.NewLego(factory, lego.WithSidecar(lego.SidecarOPA{
    Addr:      "http://127.0.0.1:8181",
    BundleDst: "/shared/filesystem/path/bundle.tar.gz",
}))
```

In this mode, whenever `generator.SetBundle` is called, it will write generated OPA bundle to `BundleDst`
