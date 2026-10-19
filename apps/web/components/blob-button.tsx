import "./blob-button.css";

import Link from "next/link";
import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";

import { cx } from "@/components/ui";

export type BlobVariant = "primary" | "light" | "onNavy";

const VARIANT: Record<BlobVariant, string> = {
  primary: "blob-btn--primary",
  light: "blob-btn--light",
  onNavy: "blob-btn--on-navy",
};

/** The four circles that rise into the fill. Shared by buttons and nav links. */
export function Blobs() {
  return (
    <span className="blob-btn__inner" aria-hidden="true">
      <span className="blob-btn__blobs">
        <span className="blob-btn__blob" />
        <span className="blob-btn__blob" />
        <span className="blob-btn__blob" />
        <span className="blob-btn__blob" />
      </span>
    </span>
  );
}

/**
 * The SVG "goo" filter the blobs melt through. Mount it once per page (the app
 * layout does); every blob button refers to it by id.
 */
export function GooFilter() {
  return (
    <svg aria-hidden="true" focusable="false" width="0" height="0" className="pointer-events-none absolute">
      <defs>
        <filter id="atlas-goo">
          <feGaussianBlur in="SourceGraphic" result="blur" stdDeviation="10" />
          <feColorMatrix in="blur" mode="matrix" values="1 0 0 0 0 0 1 0 0 0 0 0 1 0 0 0 0 0 21 -7" result="goo" />
          <feBlend in2="goo" in="SourceGraphic" result="mix" />
        </filter>
      </defs>
    </svg>
  );
}

/** A blob-effect link (navigation). */
export function BlobLink({
  href,
  variant = "light",
  className,
  children,
}: {
  href: string;
  variant?: BlobVariant;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Link href={href} className={cx("blob-btn", VARIANT[variant], className)}>
      {children}
      <Blobs />
    </Link>
  );
}

/** A blob-effect button (an action, e.g. opening a menu). */
export const BlobButton = forwardRef<
  HTMLButtonElement,
  ButtonHTMLAttributes<HTMLButtonElement> & { variant?: BlobVariant }
>(function BlobButton({ variant = "primary", className, children, type = "button", ...props }, ref) {
  return (
    <button ref={ref} type={type} {...props} className={cx("blob-btn", VARIANT[variant], className)}>
      {children}
      <Blobs />
    </button>
  );
});
