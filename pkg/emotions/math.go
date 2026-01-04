package emotions

import (
	"math"
)

// lerp performs linear interpolation between two values.
func lerp(a, b, t float64) float64 {
	return a + t*(b-a)
}

// clamp restricts a value to a range.
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// MatrixToEuler extracts roll, pitch, yaw (in radians) from a 4x4 rotation matrix.
// Uses ZYX Euler angle convention (yaw-pitch-roll).
func MatrixToEuler(m [4][4]float64) (roll, pitch, yaw float64) {
	// Extract rotation matrix elements
	r00 := m[0][0]
	r10, r11, r12 := m[1][0], m[1][1], m[1][2]
	r20, r21, r22 := m[2][0], m[2][1], m[2][2]

	// Avoid gimbal lock at pitch = ±90°
	sy := math.Sqrt(r00*r00 + r10*r10)

	var singular bool
	if sy < 1e-6 {
		singular = true
	}

	if !singular {
		roll = math.Atan2(r21, r22)
		pitch = math.Atan2(-r20, sy)
		yaw = math.Atan2(r10, r00)
	} else {
		roll = math.Atan2(-r12, r11)
		pitch = math.Atan2(-r20, sy)
		yaw = 0
	}

	return roll, pitch, yaw
}

// MatrixToHeadPose converts a 4x4 transformation matrix to a HeadPose.
func MatrixToHeadPose(m [4][4]float64) HeadPose {
	roll, pitch, yaw := MatrixToEuler(m)
	return HeadPose{
		Roll:  roll,
		Pitch: pitch,
		Yaw:   yaw,
		X:     m[0][3],
		Y:     m[1][3],
		Z:     m[2][3],
	}
}

// InterpolateMatrix performs linear interpolation between two 4x4 matrices.
// For rotation, this uses simple element-wise interpolation which works well
// for small angles. For large rotations, consider SLERP.
func InterpolateMatrix(a, b [4][4]float64, t float64) [4][4]float64 {
	var result [4][4]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			result[i][j] = lerp(a[i][j], b[i][j], t)
		}
	}

	// Re-orthonormalize the rotation part to prevent drift
	result = orthonormalize(result)

	return result
}

// orthonormalize ensures the rotation part of a 4x4 matrix is orthonormal.
func orthonormalize(m [4][4]float64) [4][4]float64 {
	// Extract rotation columns
	x := [3]float64{m[0][0], m[1][0], m[2][0]}
	y := [3]float64{m[0][1], m[1][1], m[2][1]}

	// Normalize X
	x = normalize3(x)

	// Make Y orthogonal to X, then normalize
	dot := x[0]*y[0] + x[1]*y[1] + x[2]*y[2]
	y[0] -= dot * x[0]
	y[1] -= dot * x[1]
	y[2] -= dot * x[2]
	y = normalize3(y)

	// Z = X cross Y
	z := cross3(x, y)

	// Rebuild matrix
	m[0][0], m[1][0], m[2][0] = x[0], x[1], x[2]
	m[0][1], m[1][1], m[2][1] = y[0], y[1], y[2]
	m[0][2], m[1][2], m[2][2] = z[0], z[1], z[2]

	return m
}

func normalize3(v [3]float64) [3]float64 {
	mag := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if mag < 1e-10 {
		return [3]float64{1, 0, 0}
	}
	return [3]float64{v[0] / mag, v[1] / mag, v[2] / mag}
}

func cross3(a, b [3]float64) [3]float64 {
	return [3]float64{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

// InterpolateKeyframes interpolates between two keyframes at parameter t ∈ [0, 1].
func InterpolateKeyframes(a, b Keyframe, t float64) Keyframe {
	t = clamp(t, 0, 1)

	return Keyframe{
		Head:           InterpolateMatrix(a.Head, b.Head, t),
		Antennas:       [2]float64{lerp(a.Antennas[0], b.Antennas[0], t), lerp(a.Antennas[1], b.Antennas[1], t)},
		BodyYaw:        lerp(a.BodyYaw, b.BodyYaw, t),
		CheckCollision: a.CheckCollision || b.CheckCollision,
	}
}

// KeyframeToPose converts a Keyframe to a simplified Pose.
func KeyframeToPose(kf Keyframe) Pose {
	return Pose{
		Head:     MatrixToHeadPose(kf.Head),
		Antennas: kf.Antennas,
		BodyYaw:  kf.BodyYaw,
	}
}
