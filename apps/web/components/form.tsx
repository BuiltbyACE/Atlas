/**
 * Atlas web - form primitives.
 *
 * Presentational only: they collect input and render validation hints. Every
 * rule is enforced by Go, and a refusal comes back as `ApiError` for the caller
 * to display. Shared by the Phase 2 client forms.
 */

import Link from "next/link";
import type {
  ButtonHTMLAttributes,
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from "react";

import { cx } from "@/components/ui";

export function Field({
  label,
  hint,
  children,
  htmlFor,
  className,
}: {
  label: ReactNode;
  hint?: ReactNode;
  children: ReactNode;
  htmlFor?: string;
  className?: string;
}) {
  return (
    <label className={cx("flex flex-col gap-1.5 text-sm", className)} htmlFor={htmlFor}>
      <span className="font-semibold text-night">
        {label}
        {hint && <span className="ml-1.5 font-normal text-muted">{hint}</span>}
      </span>
      {children}
    </label>
  );
}

const CONTROL =
  "w-full rounded-control border border-line-strong bg-white px-3 text-[14.5px] text-night outline-none transition placeholder:text-[#7A879C] focus:border-atlas focus:shadow-[0_0_0_3px_rgba(26,108,245,.15)] disabled:bg-[#F5F6F9] disabled:text-muted";

export function TextInput(props: InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={cx(CONTROL, "h-11", props.className)} />;
}

export function TextArea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      {...props}
      className={cx(CONTROL, "min-h-[96px] resize-y py-2.5", props.className)}
    />
  );
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={cx(CONTROL, "h-11 pr-8", props.className)} />;
}

type ButtonVariant =
  | "primary"
  | "secondary"
  | "danger"
  | "ghost"
  | "light"
  | "onNavy";
type ButtonSize = "md" | "sm";

const BUTTON_VARIANTS: Record<ButtonVariant, string> = {
  primary:
    "bg-atlas text-white shadow-action hover:bg-atlas-hover disabled:bg-atlas/50 disabled:shadow-none",
  secondary:
    "border-line-strong bg-white text-night hover:border-[#B8C3D6] hover:bg-[#FCFCFD] disabled:opacity-60",
  danger:
    "border-[#F5C2BD] bg-[#FDECEC] text-[#B42318] hover:bg-[#FBDCDA] disabled:opacity-60",
  ghost: "bg-transparent text-atlas-ink hover:bg-[#EEF3FF] disabled:opacity-60",
  light: "bg-white text-navy hover:bg-[#EAF1FF]",
  onNavy: "border-white/25 bg-white/10 text-white hover:bg-white/15",
};

const BUTTON_SIZES: Record<ButtonSize, string> = {
  md: "h-11 px-[18px] text-[14.5px] rounded-control",
  sm: "h-9 px-3 text-[13.5px] rounded-[10px]",
};

export function buttonClass(
  variant: ButtonVariant = "primary",
  size: ButtonSize = "md",
  className?: string,
): string {
  return cx(
    "inline-flex items-center justify-center gap-2 whitespace-nowrap border border-transparent font-semibold transition disabled:cursor-not-allowed",
    BUTTON_SIZES[size],
    variant === "ghost" && size === "md" && "px-2.5",
    BUTTON_VARIANTS[variant],
    className,
  );
}

export function Button({
  variant = "primary",
  size = "md",
  className,
  type = "button",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
}) {
  return (
    <button {...props} type={type} className={buttonClass(variant, size, className)} />
  );
}

/** A link styled as a button (navigation, not an action). */
export function ButtonLink({
  href,
  variant = "secondary",
  size = "md",
  className,
  children,
}: {
  href: string;
  variant?: ButtonVariant;
  size?: ButtonSize;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Link href={href} className={buttonClass(variant, size, className)}>
      {children}
    </Link>
  );
}

/** An on/off switch backed by a real checkbox (prototype `.ax-switch`). */
export function Switch({
  label,
  className,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & { label: ReactNode }) {
  return (
    <label
      className={cx(
        "relative inline-flex min-h-11 cursor-pointer items-center gap-2.5 text-[14.5px] font-medium text-night",
        className,
      )}
    >
      <input type="checkbox" {...props} className="peer absolute h-px w-px opacity-0" />
      <span
        aria-hidden="true"
        className="relative h-6 w-[42px] flex-none rounded-full bg-[#CBD3E1] transition peer-checked:bg-atlas peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-atlas after:absolute after:left-[3px] after:top-[3px] after:h-[18px] after:w-[18px] after:rounded-full after:bg-white after:shadow after:transition peer-checked:after:translate-x-[18px]"
      />
      {label}
    </label>
  );
}

export function Checkbox({
  label,
  className,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & { label: ReactNode }) {
  return (
    <label className={cx("flex items-start gap-2.5 text-sm text-night", className)}>
      <input
        type="checkbox"
        {...props}
        className="mt-0.5 h-4 w-4 flex-none rounded border-line-strong accent-atlas"
      />
      <span>{label}</span>
    </label>
  );
}
