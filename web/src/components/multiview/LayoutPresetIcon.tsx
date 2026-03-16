import type { LayoutPreset } from '../../types/multiview';

interface LayoutPresetIconProps {
  readonly preset: LayoutPreset;
  readonly size?: number;
  readonly active?: boolean;
}

function Box({ x, y, w, h }: { x: number; y: number; w: number; h: number }) {
  return <rect x={x} y={y} width={w} height={h} rx={1} fill="currentColor" opacity={0.3} stroke="currentColor" strokeWidth={0.5} />;
}

const layouts: Record<LayoutPreset, (s: number) => JSX.Element> = {
  '2-horizontal': (s) => (
    <>
      <Box x={1} y={1} w={s / 2 - 2} h={s - 2} />
      <Box x={s / 2 + 1} y={1} w={s / 2 - 2} h={s - 2} />
    </>
  ),
  '2-vertical': (s) => (
    <>
      <Box x={1} y={1} w={s - 2} h={s / 2 - 2} />
      <Box x={1} y={s / 2 + 1} w={s - 2} h={s / 2 - 2} />
    </>
  ),
  '3-columns': (s) => {
    const w = (s - 4) / 3;
    return (
      <>
        <Box x={1} y={1} w={w} h={s - 2} />
        <Box x={w + 2} y={1} w={w} h={s - 2} />
        <Box x={2 * w + 3} y={1} w={w} h={s - 2} />
      </>
    );
  },
  '3-main-side': (s) => {
    const mainW = (s - 3) * 0.66;
    const sideW = s - mainW - 3;
    const halfH = (s - 3) / 2;
    return (
      <>
        <Box x={1} y={1} w={mainW} h={s - 2} />
        <Box x={mainW + 2} y={1} w={sideW} h={halfH} />
        <Box x={mainW + 2} y={halfH + 2} w={sideW} h={halfH} />
      </>
    );
  },
  '4-grid': (s) => {
    const half = (s - 3) / 2;
    return (
      <>
        <Box x={1} y={1} w={half} h={half} />
        <Box x={half + 2} y={1} w={half} h={half} />
        <Box x={1} y={half + 2} w={half} h={half} />
        <Box x={half + 2} y={half + 2} w={half} h={half} />
      </>
    );
  },
  '5-grid': (s) => {
    const w3 = (s - 4) / 3;
    const w2 = (s - 3) / 2;
    const halfH = (s - 3) / 2;
    return (
      <>
        <Box x={1} y={1} w={w3} h={halfH} />
        <Box x={w3 + 2} y={1} w={w3} h={halfH} />
        <Box x={2 * w3 + 3} y={1} w={w3} h={halfH} />
        <Box x={1} y={halfH + 2} w={w2} h={halfH} />
        <Box x={w2 + 2} y={halfH + 2} w={w2} h={halfH} />
      </>
    );
  },
  '6-grid': (s) => {
    const w = (s - 4) / 3;
    const halfH = (s - 3) / 2;
    return (
      <>
        <Box x={1} y={1} w={w} h={halfH} />
        <Box x={w + 2} y={1} w={w} h={halfH} />
        <Box x={2 * w + 3} y={1} w={w} h={halfH} />
        <Box x={1} y={halfH + 2} w={w} h={halfH} />
        <Box x={w + 2} y={halfH + 2} w={w} h={halfH} />
        <Box x={2 * w + 3} y={halfH + 2} w={w} h={halfH} />
      </>
    );
  },
  custom: (s) => {
    const w1 = (s - 3) * 0.6;
    const w2 = s - w1 - 3;
    return (
      <>
        <Box x={1} y={1} w={w1} h={s - 2} />
        <Box x={w1 + 2} y={1} w={w2} h={s - 2} />
      </>
    );
  },
};

export function LayoutPresetIcon({ preset, size = 24, active = false }: LayoutPresetIconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      className={active ? 'text-accent-primary' : 'text-text-secondary hover:text-text-primary'}
    >
      {layouts[preset](size)}
    </svg>
  );
}
