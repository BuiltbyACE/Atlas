"use client";

import { Button } from "@/components/form";
import { Icon } from "@/components/icons";

/**
 * "Print" - the invoice document is server-rendered, so the only thing it needs
 * from the client is the browser's own print dialog. The sidebar and the top bar
 * are dropped by the `@media print` rules in `globals.css`, which leaves exactly
 * this document on the sheet: `Print` on screen, a PDF when saved from the
 * dialog (§8.4 - the invoice's PDF).
 */
export function PrintButton({ label = "Print" }: { label?: string }) {
  return (
    <Button type="button" variant="secondary" onClick={() => window.print()}>
      <Icon name="printer" className="h-4 w-4" />
      {label}
    </Button>
  );
}
