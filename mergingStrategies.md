## Motivations / Reminders
* Schema was valid before they merged. Impossibile to use a type/scalar in the schema it was not defined in.


## Object Types

* If a type is defined in one schema and not in the other, use that version as the canonical definition
* If a type is defined in 2 different schemas, merge the fields of that schema according to the following:
  * If a field is in one schema and not in the other, use that version as the canonical definition
  * If a field is in one schema and another with the same type signature, ignore it
  * If a field is in one schema and another with different signatures, return an error

## Enums

* Merge them?
* Could require validation steps at the query planning phase

## Interfaces

* If all the fields are the same, that's okay
* If not all of the fields are the same, return an error

## Unions

* Combine the two unions into one?

## Question
* What to do about conflicts in directives applied to a field that's defined in multiple schemas?
  * How would I delegate the execution of just a directive to the schema
  * For now, limit the application of directives to just ones defined in the same schema as the field.
* What to do about conflicting scalars?
  * I think they're safe to ignore since the scalar can exist in both schemas
  * could have different semantics or formatting (dates)
* What to do about argument conflicts in fields?
  * If all of the arguments are optional then its safe.
    * Query planning could validate that the combo is valid (all coming from one service) 
  * If any of the arguments aren't optional then its not safe. The schema would have to lose the required-ness of an arg
