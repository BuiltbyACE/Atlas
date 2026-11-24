import type { ChangeEvent } from "react";

import { Select, TextInput } from "@/components/form";
import type { PersonOption } from "@/lib/directory";

/**
 * Pick a person by name. The options come from the availability roster
 * (`lib/directory.ts`); when the viewer cannot read it the picker degrades to
 * the raw person-id field Go accepts, so the form still works. A current value
 * missing from the roster (an exited person, say) stays selectable.
 *
 * Controlled (`value` + `onChange`) or uncontrolled (`defaultValue`), like a
 * native select.
 */
export function PersonSelect({
  people,
  id,
  name,
  value,
  defaultValue,
  onChange,
  me,
  emptyLabel = "Unassigned",
  disabled,
  className,
}: {
  people: PersonOption[];
  id?: string;
  name?: string;
  value?: string;
  defaultValue?: string;
  onChange?: (personId: string) => void;
  /** The viewer's person id, labelled "(you)". */
  me?: string;
  emptyLabel?: string;
  disabled?: boolean;
  className?: string;
}) {
  const handle = onChange
    ? (event: ChangeEvent<HTMLSelectElement | HTMLInputElement>) =>
        onChange(event.target.value.trim())
    : undefined;

  if (people.length === 0) {
    return (
      <TextInput
        id={id}
        name={name}
        value={value}
        defaultValue={defaultValue}
        onChange={handle}
        disabled={disabled}
        placeholder="Person id (UUID) — the roster isn't visible to you"
        className={className}
      />
    );
  }

  const current = value ?? defaultValue ?? "";
  const known = current === "" || people.some((p) => p.id === current);

  return (
    <Select
      id={id}
      name={name}
      value={value}
      defaultValue={value === undefined ? defaultValue : undefined}
      onChange={handle}
      disabled={disabled}
      className={className}
    >
      <option value="">{emptyLabel}</option>
      {!known && <option value={current}>Person {current.slice(0, 8)}</option>}
      {people.map((person) => (
        <option key={person.id} value={person.id}>
          {person.name}
          {person.id === me ? " (you)" : ""}
        </option>
      ))}
    </Select>
  );
}
