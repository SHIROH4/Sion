function ThinkingDots({ visible }: { visible: boolean }) {
  if (!visible) return null

  return (
    <div style={{
      position: 'absolute',
      top: '18%',
      left: '50%',
      transform: 'translateX(-50%)',
      zIndex: 12,
      display: 'flex',
      gap: 6,
      pointerEvents: 'none',
    }}>
      {[0, 1, 2].map((i) => (
        <span
          key={i}
          style={{
            display: 'inline-block',
            width: 8,
            height: 8,
            borderRadius: '50%',
            background: '#999',
            animation: `thinking-bounce 1.4s ${i * 0.15}s infinite ease-in-out both`,
          }}
        />
      ))}
    </div>
  )
}

export default ThinkingDots
