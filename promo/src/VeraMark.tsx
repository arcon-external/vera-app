export function VeraMark({width, light = false}: {width: number; light?: boolean}) {
  const fill = light ? "#fff" : "#2B2A27";
  const accent = light ? "#fff" : "#E6D0A1";
  return <svg width={width} height={width * 300 / 409} viewBox="0 0 409 300" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path d="M0.865967 1L95.1316 1L194.366 194L179.366 222L78.366 27H46.6316L172.783 245.5L300.377 24.5H331.132L172.783 298.768L0.865967 1Z" fill={fill} stroke={fill}/>
    <path d="M300.366 24.5L314.222 0.5H407.866L235.866 298.413L207.366 250L223.366 222.946L235.866 244.751L363.028 24.5H300.366Z" fill={accent} stroke={fill}/>
    <path d="M154.866 0.760742H199.366V43.7607C199.366 65.2607 171.866 81.7607 171.866 81.7607L161.366 68.8548C163.483 70.3431 187.818 43.7607 176.366 43.7607H154.866V0.760742Z" fill={accent} stroke={fill}/>
    <path d="M214.366 0.5H258.866V43.5C258.866 65 231.366 81.5 231.366 81.5L220.866 68.594C222.983 70.0823 247.318 43.5 235.866 43.5H214.366V0.5Z" fill={accent} stroke={fill}/>
  </svg>;
}
