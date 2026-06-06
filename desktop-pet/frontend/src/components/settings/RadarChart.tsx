interface RadarChartProps {
  data: { label: string; value: number; color: string }[]
  size?: number
}

function RadarChart({ data, size = 400 }: RadarChartProps) {
  const cx = size / 2
  const cy = size / 2
  const radius = size * 0.36
  const levels = 4 // 0%, 25%, 50%, 75%, 100%
  const angleSlice = (Math.PI * 2) / data.length

  const getPoint = (i: number, r: number): [number, number] => {
    const angle = angleSlice * i - Math.PI / 2
    return [cx + r * Math.cos(angle), cy + r * Math.sin(angle)]
  }

  // Polygon points for data
  const dataPoints = data.map((d, i) => {
    const r = d.value * radius
    const [x, y] = getPoint(i, r)
    return `${x},${y}`
  }).join(' ')

  // Grid rings
  const rings = []
  for (let l = 1; l < levels; l++) {
    const r = (radius * l) / (levels - 1)
    const pts = data.map((_, i) => {
      const [x, y] = getPoint(i, r)
      return `${x},${y}`
    }).join(' ')
    rings.push(
      <polygon
        key={l}
        points={pts}
        fill="none"
        stroke="#e8e8e8"
        strokeWidth={1}
      />
    )
  }

  // Axes
  const axes = data.map((_, i) => {
    const [x, y] = getPoint(i, radius)
    return (
      <line
        key={i}
        x1={cx} y1={cy} x2={x} y2={y}
        stroke="#e8e8e8"
        strokeWidth={1}
      />
    )
  })

  // Labels
  const labels = data.map((d, i) => {
    const [x, y] = getPoint(i, radius + 22)
    return (
      <text
        key={i}
        x={x} y={y}
        textAnchor="middle"
        dominantBaseline="middle"
        fontSize={11}
        fill="#6b7280"
        style={{ fontFamily: 'inherit' }}
      >
        {d.label}
      </text>
    )
  })

  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
      {rings}
      {axes}
      <polygon
        points={dataPoints}
        fill="rgba(22,119,255,0.12)"
        stroke="#1677ff"
        strokeWidth={2}
      />
      {data.map((d, i) => {
        const r = d.value * radius
        const [x, y] = getPoint(i, r)
        return (
          <circle key={i} cx={x} cy={y} r={4} fill={d.color} stroke="#fff" strokeWidth={1.5} />
        )
      })}
      {labels}
    </svg>
  )
}

export default RadarChart
