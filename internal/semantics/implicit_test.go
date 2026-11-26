// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"sort"
	"strings"
	"testing"

	"github.com/quality-connected/sysml-v2-compiler/internal/model"
)

// impliedOf lists the implied general types of the first element of a class
// named name, as "Kind Qualified::Name".
func impliedOf(t *testing.T, w *Workspace, root *model.Element, class, name string) []string {
	t.Helper()
	var target *model.Element
	root.Walk(func(e *model.Element) {
		if target == nil && e.Class == class && e.Text("declaredName") == name {
			target = e
		}
	})
	if target == nil {
		t.Fatalf("no %s named %s", class, name)
	}
	var out []string
	for _, im := range w.ImpliedGeneralizations(target) {
		out = append(out, im.Kind+" "+QualifiedName(im.Target))
	}
	sort.Strings(out)
	return out
}

func TestImpliedGeneralTypes(t *testing.T) {
	w, roots := resolved(t, true, `package P {
		part def Car;
		item def Box { item inner : Box; connection link connect inner to inner; }
		part car : Car;
	}`)
	cases := []struct {
		class, name string
		want        []string
	}{
		{"PartUsage", "car", []string{"Subsetting Parts::parts"}},
		{"ItemUsage", "inner", []string{"Subsetting Items::Item::subitems"}},
		{"ConnectionUsage", "link", []string{"Subsetting Connections::binaryConnections", "Subsetting Items::Item::subparts"}},
	}
	for _, c := range cases {
		got := impliedOf(t, w, roots[0], c.class, c.name)
		if strings.Join(got, "; ") != strings.Join(c.want, "; ") {
			t.Errorf("%s %s: implied %q, want %q", c.class, c.name, got, c.want)
		}
	}
}

func TestMaterializeWritesTheImpliedRelationshipsIntoTheTree(t *testing.T) {
	w, roots := resolved(t, true, `package P {
		part def Car;
		part car : Car;
		attribute n : ScalarValues::Integer = 3;
	}`)
	count := func() (implied, all int) {
		roots[0].Walk(func(e *model.Element) {
			all++
			if e.IsTrue("isImplied") {
				implied++
			}
		})
		return
	}
	var carBefore []string
	car := w.Lookup("P::car")
	for _, s := range w.Supertypes(car) {
		carBefore = append(carBefore, QualifiedName(s))
	}
	if n, _ := count(); n != 0 {
		t.Fatalf("%d implied elements before Materialize", n)
	}
	w.Materialize(roots...)
	implied1, all1 := count()
	if implied1 == 0 {
		t.Fatal("Materialize wrote nothing")
	}
	// the typing of n, and the subsetting of car, are there to be found
	var subsets []string
	car.Walk(func(e *model.Element) {
		if e.IsTrue("isImplied") && e.IsA("Subsetting") && e.Owner == car {
			subsets = append(subsets, QualifiedName(e.Refs("subsettedFeature")[0].Target))
		}
	})
	if len(subsets) != 1 || subsets[0] != "Parts::parts" {
		t.Errorf("car's implied subsettings: %v", subsets)
	}
	w.Materialize(roots...)
	if implied2, all2 := count(); implied2 != implied1 || all2 != all1 {
		t.Errorf("a second Materialize changed the tree: %d,%d -> %d,%d", implied1, all1, implied2, all2)
	}
	// what the queries say does not change because the tree now holds it
	var carAfter []string
	for _, s := range w.Supertypes(car) {
		carAfter = append(carAfter, QualifiedName(s))
	}
	if strings.Join(carBefore, ",") != strings.Join(carAfter, ",") {
		t.Errorf("Supertypes changed: %v -> %v", carBefore, carAfter)
	}
	if got := w.ImpliedGeneralizations(car); len(got) != 1 {
		t.Errorf("ImpliedGeneralizations after Materialize: %d", len(got))
	}
}

// assign index := 1 : the target's nested features redefine startingAt and
// accessedFeature (the latter also the feature assigned), and the value
// redefines replacementValues.
func TestImpliedRedefinitionsOfAnAssignment(t *testing.T) {
	w, roots := resolved(t, true, `package P {
		action def Loop { attribute index; action init assign index := 1; }
	}`)
	var assign *model.Element
	roots[0].Walk(func(e *model.Element) {
		if assign == nil && e.Class == "AssignmentActionUsage" {
			assign = e
		}
	})
	if assign == nil {
		t.Fatal("no assignment")
	}
	redefines := func(f *model.Element) string {
		var out []string
		for _, im := range w.ImpliedGeneralizations(f) {
			if im.Kind == "Redefinition" {
				out = append(out, QualifiedName(im.Target))
			}
		}
		sort.Strings(out)
		return strings.Join(out, "; ")
	}
	params := w.ownedParameters(assign)
	if len(params) != 2 {
		t.Fatalf("%d parameters, want 2", len(params))
	}
	startingAt := w.ownedFeatures(params[0])[0]
	accessed := w.ownedFeatures(startingAt)[0]
	for _, c := range []struct {
		name string
		f    *model.Element
		want string
	}{
		{"startingAt", startingAt, "FeatureReferencingPerformances::FeatureAccessPerformance::onOccurrence::startingAt"},
		{"accessedFeature", accessed, "FeatureReferencingPerformances::FeatureAccessPerformance::onOccurrence::startingAt::accessedFeature; P::Loop::index"},
		{"value", params[1], "Actions::AssignmentAction::replacementValues"},
	} {
		if got := redefines(c.f); got != c.want {
			t.Errorf("%s redefines %q, want %q", c.name, got, c.want)
		}
	}
}
