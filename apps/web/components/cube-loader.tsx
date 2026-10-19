import "./cube-loader.css";

import { cx } from "@/components/ui";

const AXIS = [1, 2, 3] as const;

/**
 * The cube loader: a 3×3×3 block of isometric cubes that splits apart and
 * reassembles (Paul Grant's "Cube loading animation" on CodePen). The faces
 * use the Atlas palette; the motion lives in `cube-loader.css`.
 *
 * It announces itself as a status with a text label, so a screen reader hears
 * "Loading …" rather than 81 empty boxes.
 */
export function CubeLoader({
  label = "Loading",
  scale = 0.5,
  className,
}: {
  label?: string;
  /** Size relative to the 86×100 px original. */
  scale?: number;
  className?: string;
}) {
  return (
    <div role="status" aria-live="polite" className={cx("flex flex-col items-center gap-5", className)}>
      <div aria-hidden="true" style={{ width: 86 * scale, height: 100 * scale }} className="grid place-items-center">
        <div className="cube-loader" style={{ ["--cube-scale" as string]: scale }}>
          {AXIS.map((h) =>
            AXIS.map((w) =>
              AXIS.map((l) => (
                <div key={`${h}${w}${l}`} className={`cube-loader__cube h${h} w${w} l${l}`}>
                  <div className="cube-loader__face cube-loader__face--top" />
                  <div className="cube-loader__face cube-loader__face--left" />
                  <div className="cube-loader__face cube-loader__face--right" />
                </div>
              )),
            ),
          )}
        </div>
      </div>
      <span className="text-sm font-medium text-muted">{label}…</span>
    </div>
  );
}
