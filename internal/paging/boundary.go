// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package paging

// EitherMinID returns an ID boundary with given min ID value,
// using either the `since_id`,"DESC" name,ordering or
// `min_id`,"ASC" name,ordering depending on which is set.
func EitherMinID(minID, sinceID string) Boundary {
	/*

				Paging with `since_id` vs `min_id`:

					limit = 4       limit = 4
					+----------+    +----------+
		  max_id--> |xxxxxxxxxx|    |          | <-- max_id
					+----------+    +----------+
					|xxxxxxxxxx|    |          |
					+----------+    +----------+
					|xxxxxxxxxx|    |          |
					+----------+    +----------+
					|xxxxxxxxxx|    |xxxxxxxxxx|
					+----------+    +----------+
					|          |    |xxxxxxxxxx|
					+----------+    +----------+
					|          |    |xxxxxxxxxx|
					+----------+    +----------+
		since_id--> |          |    |xxxxxxxxxx| <-- min_id
					+----------+    +----------+
					|          |    |          |
					+----------+    +----------+

				To sum it up in words:

				when paging with since_id, max_id is used as
				the cursor value, and since_id provides a
				limiting value to the results.

				when paging with min_id, min_id is used as
				the cursor value, and max_id provides a
				limiting value to the results.

				But to further complicate it...

				The "next" and "prev" relative links provided
				in the link header are ALWAYS DESCENDING. Which
				means we will ALWAYS provide next=?max_id and
				prev=?min_id. *shakes fist at mastodon api*

	*/
	switch {
	case minID != "":
		return MinID(minID)
	default:
		// default min is `since_id`
		return SinceID(sinceID)
	}
}

// SinceID ...
func SinceID(sinceID string) Boundary {
	return Boundary{
		// even when a since_id query is
		// provided, the next / prev rel
		// links are DESCENDING with
		// next:max_id and prev:min_id.
		// so ALWAYS use min_id as name.
		Name:  "min_id",
		Value: sinceID,
		Order: OrderDescending,
	}
}

// MinID ...
func MinID(minID string) Boundary {
	return Boundary{
		Name:  "min_id",
		Value: minID,
		Order: OrderAscending,
	}
}

// MaxID returns an ID boundary with given max
// ID value, and the "max_id" query key set.
func MaxID(maxID string) Boundary {
	return Boundary{
		Name:  "max_id",
		Value: maxID,
		Order: OrderDescending,
	}
}

// MinShortcodeDomain returns a boundary with the given minimum emoji
// shortcode@domain, and the "min_shortcode_domain" query key set.
func MinShortcodeDomain(min string) Boundary {
	return Boundary{
		Name:  "min_shortcode_domain",
		Value: min,
		Order: OrderAscending,
	}
}

// MaxShortcodeDomain returns a boundary with the given maximum emoji
// shortcode@domain, and the "max_shortcode_domain" query key set.
func MaxShortcodeDomain(max string) Boundary {
	return Boundary{
		Name:  "max_shortcode_domain",
		Value: max,
		Order: OrderDescending,
	}
}

// Boundary represents the upper or lower limit in a page slice.
type Boundary struct {
	Name  string // i.e. query key
	Value string
	Order Order // NOTE: see Order type for explanation
}

// new creates a new Boundary with the same ordering and name
// as the original (receiving), but with the new provided value.
func (b Boundary) new(value string) Boundary {
	return Boundary{
		Name:  b.Name,
		Value: value,
		Order: b.Order,
	}
}

// Find finds the boundary's set value in input slice, or returns -1.
func (b Boundary) Find(in []string) int {
	if b.Value == "" {
		return -1
	}
	for i := range in {
		if in[i] == b.Value {
			return i
		}
	}
	return -1
}

// Boundary_FindFunc is functionally equivalent to Boundary{}.Find() but for an arbitrary type with ID.
// Note: this is not a Boundary{} method as Go generics are not supported in method receiver functions.
func Boundary_FindFunc[T any](b Boundary, in []T, get func(T) string) int { //nolint:revive
	if get == nil {
		panic("nil function")
	}
	if b.Value == "" {
		return -1
	}
	for i := range in {
		if get(in[i]) == b.Value {
			return i
		}
	}
	return -1
}
