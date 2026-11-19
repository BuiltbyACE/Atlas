import { CubeLoader } from "@/components/cube-loader";

/**
 * Shown the moment a workspace link is followed, while the next screen's
 * server components read from Go. The sidebar and top bar stay put; only the
 * page area shows the loader. Every route under (app) inherits it.
 */
export default function Loading() {
  return (
    <div className="grid min-h-[60vh] place-items-center">
      <CubeLoader scale={0.75} />
    </div>
  );
}
