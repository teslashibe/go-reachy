package zenohclient

import "fmt"

// Topic constants for Reachy Mini Zenoh communication.
// All topics are prefixed with the configured prefix (default: "reachy_mini").

// TopicCommand is the motor command topic.
// Publishes: JSON with head_pose, antennas, body_yaw
const TopicCommand = "command"

// TopicJointPositions is the joint position feedback topic.
// Subscribes: JSON with current joint positions
const TopicJointPositions = "joint_positions"

// TopicHeadPose is the head pose feedback topic.
// Subscribes: JSON with current head pose matrix
const TopicHeadPose = "head_pose"

// TopicDaemonStatus is the daemon status topic.
// Subscribes: JSON with robot daemon status
const TopicDaemonStatus = "daemon_status"

// TopicTask is the task request topic.
// Publishes: JSON with task requests
const TopicTask = "task"

// TopicTaskProgress is the task progress feedback topic.
// Subscribes: JSON with task progress updates
const TopicTaskProgress = "task_progress"

// TopicRecordedData is the recorded data topic.
// Subscribes: Binary data for recordings
const TopicRecordedData = "recorded_data"

// Audio topics (new for this feature)

// TopicAudioMic is the microphone audio stream topic.
// Publishes: PCM16 audio chunks from the robot's microphone
const TopicAudioMic = "audio/mic"

// TopicAudioSpeaker is the speaker audio stream topic.
// Subscribes: PCM16 audio chunks to play on the robot's speaker
const TopicAudioSpeaker = "audio/speaker"

// TopicAudioDOA is the direction of arrival topic.
// Publishes: JSON with DOA angle and speaking detection
const TopicAudioDOA = "audio/doa"

// Topics is a helper to build fully-qualified topic names.
type Topics struct {
	prefix string
}

// NewTopics creates a Topics helper with the given prefix.
func NewTopics(prefix string) *Topics {
	return &Topics{prefix: prefix}
}

// Command returns the full command topic path.
func (t *Topics) Command() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicCommand)
}

// JointPositions returns the full joint positions topic path.
func (t *Topics) JointPositions() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicJointPositions)
}

// HeadPose returns the full head pose topic path.
func (t *Topics) HeadPose() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicHeadPose)
}

// DaemonStatus returns the full daemon status topic path.
func (t *Topics) DaemonStatus() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicDaemonStatus)
}

// Task returns the full task topic path.
func (t *Topics) Task() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicTask)
}

// TaskProgress returns the full task progress topic path.
func (t *Topics) TaskProgress() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicTaskProgress)
}

// RecordedData returns the full recorded data topic path.
func (t *Topics) RecordedData() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicRecordedData)
}

// AudioMic returns the full audio mic topic path.
func (t *Topics) AudioMic() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicAudioMic)
}

// AudioSpeaker returns the full audio speaker topic path.
func (t *Topics) AudioSpeaker() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicAudioSpeaker)
}

// AudioDOA returns the full audio DOA topic path.
func (t *Topics) AudioDOA() string {
	return fmt.Sprintf("%s/%s", t.prefix, TopicAudioDOA)
}

